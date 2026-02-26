# LSM Tree — Проектная документация

## Обзор

LSM Tree (Log-Structured Merge Tree) — key-value хранилище, оптимизированное для записи. Данные сначала накапливаются в памяти (memtable), затем сбрасываются на диск в виде отсортированных SST-файлов (Sorted String Table). Периодически файлы на одном уровне объединяются (compaction) и записываются на следующий уровень.

Реализация — однопоточная, без блокировок.

## Ключевое архитектурное решение

### Подключаемая стратегия слияния

Единственное отличие между KV-хранилищем и хранилищем инвертированного индекса — логика слияния при compaction:

| Режим | При одинаковых ключах | Используется |
|-------|----------------------|-------------|
| KV (по умолчанию) | Побеждает последняя запись | `MergeIterator` |
| Bitmap | OR-объединение Roaring Bitmap | `BitmapMergeIterator` |

Стратегия инжектируется через `config.MergeIteratorFactory`:

```
config.Config
  └── MergeIteratorFactory func([]SubIterator) MergeIterator
         │
         ├── nil (по умолчанию) → MergeIterator (last-write-wins)
         └── BitmapMergeFactory() → BitmapMergeIterator (OR merge)
```

Это позволяет **одной реализации Tree** обслуживать оба сценария без дублирования кода.

## Компоненты

### Диаграмма зависимостей

```
config          (интерфейсы: MergeIterator, SubIterator, Hooks)
  ↑
iterator        (реализации: MergeIterator, BitmapMergeIterator, фабрики)
  ↑
memtable        (TreeMap, HashMap — реализации MemTable)
  ↑
sst_io/writer   (запись SST: data blocks → filter → index → footer)
sst_io/reader   (чтение SST: footer → index → filter → data block)
  ↑
node            (обёртка над SST-файлом: Get, GetRange, bloom filter)
  ↑
tree            (Tree: Put/Get/Delete/Scan, flush, compaction)
```

Нет циклических зависимостей. `config` определяет только интерфейсы и не импортирует внутренние пакеты.

## Memtable

### Интерфейс

```go
type MemTable interface {
    Put(key string, value []byte)
    Get(key string) ([]byte, bool)
    All() []*KV              // все пары, отсортированные по ключу
    Size() int               // текущий размер в байтах
    KvsCnt() int             // количество KV-пар
}
```

### Реализации

| Реализация | Структура | All() | Put/Get |
|-----------|-----------|-------|---------|
| `TreeMapMemTable` | Red-Black Tree | O(n) упорядоченный обход | O(log n) |
| `MapMemTable` | Go map + sort | O(n log n) сортировка при вызове | O(1) amortized |

По умолчанию используется `TreeMapMemTable` — гарантирует порядок без дополнительной сортировки при flush.

### Жизненный цикл

```
1. Put(key, value) → запись в memtable
2. memtable.Size() >= SSTSize → flush
3. Создаётся новый пустой memtable
4. Старый memtable передаётся в flushMemTable()
```

## SST-файл

### Бинарная структура

```
Offset 0
┌────────────────────────────────────────┐
│           Data Block 0                  │
│  ┌─────────────────────────────────┐   │
│  │ key_len(4B) │ key │ val_len(4B) │   │
│  │ value                           │   │
│  │ key_len(4B) │ key │ val_len(4B) │   │
│  │ value                           │   │
│  │ ...                             │   │
│  └─────────────────────────────────┘   │
├────────────────────────────────────────┤
│           Data Block 1                  │
│  (аналогичная структура)               │
├────────────────────────────────────────┤
│           ...                           │
├────────────────────────────────────────┤
│           Data Block N                  │
├────────────────────────────────────────┤
│                                        │
│           Filter Block                  │
│  ┌─────────────────────────────────┐   │
│  │ block_offset(8B) │ bitmap_len   │   │
│  │ bloom_filter_bitmap             │   │
│  │ ...                             │   │
│  └─────────────────────────────────┘   │
│                                        │
├────────────────────────────────────────┤
│                                        │
│           Index Block                   │
│  ┌─────────────────────────────────┐   │
│  │ key_len(4B) │ max_key           │   │ ← макс. ключ блока
│  │ prev_block_offset(8B)           │   │ ← смещение блока данных
│  │ prev_block_size(8B)             │   │ ← размер блока данных
│  │ ...                             │   │
│  └─────────────────────────────────┘   │
│                                        │
├────────────────────────────────────────┤
│           Footer (32 байта)             │
│  ┌─────────────────────────────────┐   │
│  │ filter_block_offset(8B)         │   │
│  │ filter_block_size(8B)           │   │
│  │ index_block_offset(8B)          │   │
│  │ index_block_size(8B)            │   │
│  └─────────────────────────────────┘   │
└────────────────────────────────────────┘
```

### Именование файлов

```
{level}_{seq}.sst

Примеры:
  0_1.sst   — Level 0, первый SST
  0_2.sst   — Level 0, второй SST
  1_1.sst   — Level 1, первый SST (результат compaction)
```

`seq` монотонно возрастает и не сбрасывается после compaction.

### Запись SST (SSTWriter)

```
1. Append(key, value)
   → Добавить KV в текущий блок
   → Добавить key в bloom filter текущего блока
   → Если размер блока >= SSTDataBlockSize:
       → Записать блок на диск
       → Сохранить bloom filter bitmap
       → Создать index entry (max_key, offset, size)
       → Начать новый блок и новый bloom filter

2. Finish()
   → Дописать последний блок
   → Записать filter block (все bloom bitmap)
   → Записать index block (все index entries)
   → Записать footer (смещения filter и index)
   → Вернуть (size, blockToFilter, index)
```

### Чтение SST (SSTReader)

```
Открытие файла:
  1. Прочитать footer (последние 32 байта)
  2. Прочитать index block → массив Index entries
  3. Прочитать filter block → map[offset]bitmap

Точечное чтение (Get):
  1. Binary search по index → найти блок, где может быть ключ
  2. Bloom filter проверка → если ключ отсутствует, вернуть false
  3. Прочитать блок → линейный поиск по KV-парам

Диапазонное чтение (GetRange):
  1. Проверить пересечение [startKey, endKey) с [startKey, endKey] SST
  2. Binary search по index → найти первый блок
  3. Последовательно читать блоки до выхода за endKey
```

## Node

Node — обёртка над одним SST-файлом. Хранит:

```go
type Node struct {
    conf          *config.Config
    file          string              // имя файла (без пути)
    level         int
    seq           int32
    size          uint64
    blockToFilter map[uint64][]byte   // offset → bloom bitmap
    index         []*writer.Index     // отсортированный по ключу
    startKey      []byte              // минимальный ключ в SST
    endKey        []byte              // максимальный ключ в SST
    sstReader     *reader.SSTReader
}
```

### Оптимизация GetRange

До оптимизации: `GetAll()` → фильтрация. Читает все блоки.

После оптимизации:
```
1. Проверка: startKey > endKey SST → пропустить
2. Binary search: найти первый блок с maxKey >= startKey
3. Читать блоки последовательно, пока maxKey блока < endKey
4. Остановиться, когда maxKey блока >= endKey
```

При 100K ключах и 1000 features запрос одного feature читает 1–3 блока вместо всех (~100x ускорение).

## Tree

### Интерфейс

```go
type LSMTree interface {
    Put(key string, value []byte) error
    Get(key string) ([]byte, bool, error)
    Delete(key string) error
    Scan(startKey, endKey string) ([]*KVResult, error)
    Close()
}
```

### Внутреннее состояние

```go
type Tree struct {
    Conf       *config.Config
    memTable   memtable.MemTable
    nodes      [][]*node.Node      // nodes[level] — SST на уровне
    levelToSeq []atomic.Int32      // счётчик seq для каждого уровня
    stats      Stats
}
```

### Put

```
Put(key, value)
  │
  ├── memTable.Put(key, value)
  ├── [Hook: OnWrite]
  │
  └── if memTable.Size() >= SSTSize:
        ├── flushMemTable(memTable)
        └── memTable = new MemTable
```

### Get

```
Get(key)
  │
  ├── memTable.Get(key)     ← если найдено (включая tombstone), вернуть
  │
  └── for level 0..N:
        for node (новые → старые):
          ├── bloom filter → пропустить, если ключ отсутствует
          ├── binary search index → найти блок
          └── прочитать блок → линейный поиск
              └── если найдено (включая tombstone), вернуть
```

### Delete

Запись tombstone: `Put(key, nil)`. При чтении `nil` значение интерпретируется как удалённый ключ.

### Scan

```
Scan(startKey, endKey)
  │
  ├── for level N..0 (снизу вверх):
  │     for each node:
  │       GetRange(startKey, endKey) → merged map (верхние уровни перезаписывают)
  │
  ├── memTable.All() → фильтр по диапазону → добавить в merged map
  │
  └── Убрать tombstone (value == nil), отсортировать, вернуть
```

## Flush

```
flushMemTable(memTable)
  │
  ├── kvs = memTable.All()
  ├── seq = levelToSeq[0].Add(1)
  ├── SSTWriter.Append(key, value) для каждого KV
  ├── SSTWriter.Finish() → (size, blockToFilter, index)
  │
  ├── insertNode(level=0, seq, size, blockToFilter, index)
  ├── stats.FlushCount++
  ├── [Hook: OnFlush]
  │
  └── cascadeCompact()
```

## Compaction

### Каскадная проверка

```
cascadeCompact()
  for level 0..MaxLevel-2:
    if len(nodes[level]) >= SSTNumPerLevel:
      compactLevel(level)
    else:
      break    ← если уровень не переполнен, дальше не проверять
```

### Compaction одного уровня

```
compactLevel(level)
  │
  ├── Создать SubIterator для каждого SST на уровне
  │
  ├── if MergeIteratorFactory != nil:
  │     mergeIter = MergeIteratorFactory(subs)    ← инжектированная стратегия
  │   else:
  │     mergeIter = NewMergeIterator(iters)       ← KV по умолчанию
  │
  ├── SSTWriter.Append() для каждого элемента из mergeIter
  ├── SSTWriter.Finish()
  │
  ├── insertNode(level+1, ...)
  ├── removeNodes(level, oldNodes)    ← Close + удалить файлы
  ├── stats.CompactCount++
  └── [Hook: OnCompaction]
```

### MergeIterator (KV-режим)

Многопутевое слияние через min-heap:

```
Heap: отсортирован по (key ASC, idx DESC)
  idx — порядок итератора, больший idx = более новые данные

Next():
  1. Pop минимальный элемент (самый маленький key, самый новый)
  2. Сохранить key + value
  3. Пропустить все элементы в heap с таким же key (устаревшие версии)
  4. Продвинуть соответствующие итераторы
```

### BitmapMergeIterator (Bitmap-режим)

```
Next():
  1. Pop минимальный элемент
  2. Десериализовать value → Roaring Bitmap
  3. Пока в heap есть элементы с таким же key:
       Pop → десериализовать → OR-объединить
  4. Сериализовать объединённый bitmap → value
```

## Bloom Filter

### Параметры

- Размер bitmap: 10240 бит (≈ 1.25 KB)
- Количество хеш-функций: зависит от реализации
- Один фильтр на каждый data block

### Использование

```
Запись (SSTWriter):
  Для каждого key в блоке → filter.Add(bitmap, key)
  При завершении блока → сохранить bitmap в blockToFilter

Чтение (Node.Get):
  1. Binary search → найти index entry с нужным блоком
  2. bitmap = blockToFilter[entry.PrevBlockOffset]
  3. filter.Exist(bitmap, key) → false = точно нет, true = возможно есть
```

## Наблюдаемость

### Stats

```go
type Stats struct {
    BytesWritten uint64    // байт записано на диск
    BytesRead    uint64    // байт прочитано с диска
    FlushCount   int       // количество flush
    CompactCount int       // количество compaction
}

// Дополнительно:
tree.SSTPerLevel() []int   // количество SST на каждом уровне
tree.MemTableSize() int    // текущий размер memtable
tree.MemTableCount() int   // количество KV в memtable
```

### Hooks

```go
type Hooks struct {
    OnFlush      func(FlushInfo)      // после каждого flush
    OnCompaction func(CompactionInfo)  // после каждого compaction
    OnWrite      func(key []byte, valueSize int)  // после каждого Put
}
```

Все поля опциональны (`nil` = не вызывать).

## Конфигурация

### Параметры

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|-------------|----------|
| `Dir` | string | обязательный | Директория для SST-файлов |
| `MaxLevel` | int | 4 | Максимальное количество уровней |
| `SSTSize` | int | 4 MB | Порог сброса memtable (байт) |
| `SSTNumPerLevel` | int | 10 | SST на уровне до compaction |
| `SSTDataBlockSize` | int | 4 KB | Размер data block в SST |
| `SSTFooterSize` | int | 32 | Размер footer (фиксированный) |
| `Filter` | filter.Filter | BloomFilter(10240) | Фильтр для чтения |
| `FilterConstructor` | func() Filter | BloomFilter(10240) | Фабрика фильтров для записи |
| `MemTableConstructor` | func() MemTable | TreeMapMemTable | Фабрика memtable |
| `MergeIteratorFactory` | func | nil (KV) | Стратегия слияния |
| `Hooks` | Hooks | {} | Callback-и событий |

### Рекомендации по настройке

**Много мелких записей** (словарь терминов):
```go
config.WithSSTSize(1 * 1024 * 1024),    // 1 MB — чаще flush
config.WithSSTNumPerLevel(5),            // чаще compact
```

**Большие значения** (bitmap-ы инвертированного индекса):
```go
config.WithSSTSize(8 * 1024 * 1024),    // 8 MB — реже flush
config.WithSSTNumPerLevel(10),           // реже compact
config.WithSSTDataBlockSize(8 * 1024),   // 8 KB блоки
```

**Отладка / наблюдение**:
```go
config.WithHooks(config.Hooks{
    OnFlush:      func(info config.FlushInfo) { log.Printf(...) },
    OnCompaction: func(info config.CompactionInfo) { log.Printf(...) },
})
```