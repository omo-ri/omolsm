# Инвертированный индекс — Проектная документация

## Обзор

Полнотекстовый поисковый движок с двумя режимами хранения (память / LSM) и поддержкой булевых запросов. Работает поверх LSM Tree с bitmap-стратегией слияния.

### Компоненты

```
engine          ← точка входа: индексация файлов, поиск
  ├── textproc  ← токенизация, нормализация, стемминг, стоп-слова
  ├── parser    ← разбор булевых запросов в AST
  ├── dictionary← отображение term → featureID
  └── index     ← отображение featureID → Roaring Bitmap (docID-ы)
```

## Текстовая обработка (textproc)

### Pipeline

Текст проходит через цепочку обработчиков:

```
Входной текст
  → Tokenizer       : разбиение на слова (Unicode-aware)
  → LanguageDetector : определение языка (english / russian / mixed)
  → Normalizer       : lowercase, удаление акцентов, фильтр по длине
  → StopWordFilter   : удаление стоп-слов (по языку)
  → Stemmer          : стемминг (Snowball, по языку)
  → []string         : список нормализованных термов
```

### Языки

| Язык | Стемминг | Стоп-слова | Детекция |
|------|---------|-----------|---------|
| English | Snowball English | ~175 слов | Латинские символы |
| Russian | Snowball Russian | ~250 слов | Кириллические символы |
| Mixed | Оба | Оба | По символу каждого слова |
| Auto | Автоопределение | По определённому языку | Анализ первых N слов |

### Важно: индексация и поиск через один Pipeline

Стемминг, нормализация и стоп-слова должны совпадать между `index` и `query` pipeline-ами:

```yaml
index:
  stemming: true      # ← должны совпадать
  normalize: true     # ← должны совпадать

query:
  stemming: true      # ← должны совпадать
  normalize: true     # ← должны совпадать
  stop_words: false   # ← может отличаться (для query обычно выключены)
```

Если включить стемминг при индексации и выключить при поиске, термы не совпадут.

## Dictionary

### Интерфейс

```go
type Dictionary interface {
    GetOrAdd(term string) uint32
    Get(term string) (uint32, bool)
    GetOrAddTerms(terms []string) []uint32
    Size() int
}
```

### MemDictionary

```
term → featureID:  map[string]uint32
featureID → term:  []string (индекс = featureID)
```

O(1) по всем операциям.

### LSMDictionary

```
Ключ: term (строка)
Значение: featureID (4 байта, big-endian)
```

Использует `tree.Tree` в KV-режиме (стратегия слияния по умолчанию — перезапись). При compaction дублирующиеся термы автоматически дедуплицируются.

### Размещение SST

```
.lsm-data/
  └── dictionary/
        ├── 0_21.sst
        ├── 0_22.sst
        ├── 1_1.sst
        └── 1_2.sst
```

## InvertedIndex

### Интерфейс

```go
type InvertedIndex interface {
    Add(docID uint32, featureIDs []uint32)
    GetPostingList(featureID uint32) *roaring.Bitmap
    And(featureIDs []uint32) *roaring.Bitmap
    DocCount() uint32
    FeatureCount() int
}
```

### MemIndex

```go
type MemIndex struct {
    postings map[uint32]*roaring.Bitmap  // featureID → bitmap
}
```

Прямое отображение в памяти. O(1) доступ.

### LSMIndex

Тонкая обёртка над `tree.Tree` с bitmap-стратегией слияния.

#### Кодирование ключа

```
┌────────────────┬────────────────┐
│  featureID     │   blockID      │
│  4 байта BE    │   4 байта BE   │
└────────────────┴────────────────┘
          8 байт, big-endian
```

- **featureID**: идентификатор термина из Dictionary
- **blockID**: `docID / blockSize` (по умолчанию blockSize = 1024)

Big-endian обеспечивает: лексикографический порядок = числовой порядок. Все записи одного featureID физически смежны в SST.

#### Значение

Сериализованный Roaring Bitmap (`bitmap.ToBytes()`), содержащий сырые docID.

#### Запись (Add)

```
Add(docID=5000, featureIDs=[42, 17])
  │
  ├── blockID = 5000 / 1024 = 4
  │
  ├── Для featureID=42:
  │     key = encode(42, 4) = [0x00 0x00 0x00 0x2A 0x00 0x00 0x00 0x04]
  │     old = tree.Get(key) → десериализовать bitmap
  │     old.Add(5000)
  │     tree.Put(key, old.ToBytes())
  │
  └── Для featureID=17: (аналогично)
```

#### Чтение (GetPostingList)

```
GetPostingList(featureID=42)
  │
  ├── startKey = encode(42, 0)
  ├── endKey   = encode(43, 0)       ← эксклюзивная верхняя граница
  │
  ├── kvs = tree.Scan(startKey, endKey)
  │     → возвращает все (42, blockID=0), (42, blockID=1), ...
  │
  ├── result = new Bitmap
  ├── for each kv:
  │     bm = deserialize(kv.Value)
  │     result.Or(bm)
  │
  └── return result                   ← все docID для featureID=42
```

#### Compaction

LSM Tree автоматически выполняет compaction с `BitmapMergeIterator`:

```
Два SST на Level 0:
  SST-A: (42, 0) → bitmap{100, 200}
  SST-B: (42, 0) → bitmap{200, 300}

После compaction на Level 1:
  SST-C: (42, 0) → bitmap{100, 200, 300}   ← OR-объединение
```

Данные не теряются — bitmap-ы объединяются, а не перезаписываются.

#### Размещение SST

```
.lsm-data/
  └── index/
        ├── 1_1.sst
        ├── 1_2.sst
        └── 1_3.sst
```

## Parser

### Грамматика (BNF)

```
expr    → orExpr
orExpr  → andExpr ("OR" andExpr)*
andExpr → primary ("AND" ["NOT"] primary)*
primary → "(" expr ")" | TERM
```

### Приоритет операторов

| Приоритет | Оператор | Описание |
|-----------|---------|---------|
| 1 (высший) | `()` | Группировка |
| 2 | `AND`, `AND NOT` | Пересечение, исключение |
| 3 (низший) | `OR` | Объединение |

### Примеры разбора

```
"fox AND dog"
  → AND(Search("fox"), Search("dog"))

"fox OR cat"
  → OR(Search("fox"), Search("cat"))

"(fox OR cat) AND NOT snake"
  → AND_NOT(OR(Search("fox"), Search("cat")), Search("snake"))
```

### Токенизация

- Разделители: пробелы, табы
- Скобки `(` `)` — отдельные токены
- `AND`, `OR`, `NOT` — регистронезависимые ключевые слова
- Всё остальное — термины

## Engine

### Цепочка индексации

```
IndexFile(path)
  │
  ├── Прочитать файл → text
  ├── docID = nextID++
  ├── Сохранить DocInfo{ID, Filename}
  │
  ├── pipeline.Process(text) → []terms
  ├── dictionary.GetOrAddTerms(terms) → []featureIDs
  └── index.Add(docID, featureIDs)
```

### Цепочка поиска

```
Search("fox", "dog")     ← AND по умолчанию
  │
  ├── Для каждого term:
  │     queryPipeline.Process(term) → normalized terms
  │     dictionary.Get(term) → featureID (или skip, если не найден)
  │
  ├── Если один featureID:
  │     index.GetPostingList(featureID) → bitmap
  │
  ├── Если несколько featureIDs:
  │     index.And(featureIDs) → bitmap (пересечение)
  │
  └── return Result{bitmap, engine}
```

### Result — цепочка операций

```go
// Строковые методы (покрывают 99% случаев)
result.And("term1", "term2")     // AND с новыми терминами
result.Or("term1")               // OR
result.Not("term1")              // AND NOT

// Result-based (для вложенных запросов)
result.AndResult(other)          // AND двух Result
result.OrResult(other)           // OR двух Result
result.NotResult(other)          // AND NOT двух Result

// Доступ к данным
result.IsEmpty() bool
result.Count() uint64
result.DocIDs() []uint32
result.Documents() []DocInfo
```

Каждый метод возвращает **новый** Result — оригинал не мутируется.

### Связь Parser ↔ Engine

```
parser.Execute(engine, "(fox OR cat) AND NOT snake")
  │
  ├── tokenize → ["(", "fox", "OR", "cat", ")", "AND", "NOT", "snake"]
  │
  ├── parsePrimary("fox") → engine.Search("fox") → Result A
  ├── parsePrimary("cat") → engine.Search("cat") → Result B
  ├── parseOr: A.OrResult(B) → Result C
  │
  ├── parsePrimary("snake") → engine.Search("snake") → Result D
  ├── parseAnd: C.NotResult(D) → Result E
  │
  └── return Result E
```

## Режимы хранения

### Сравнение

| | Memory | LSM |
|---|--------|-----|
| Персистентность | Нет | Да (SST-файлы) |
| Потребление RAM | Всё в памяти | memtable + кеш |
| Скорость записи | Очень быстрая | Быстрая (sequential I/O) |
| Скорость чтения | O(1) HashMap | O(log n) + disk I/O |
| Compaction | Не нужен | Автоматический |
| Когда использовать | Мало данных, не нужна персистентность | Много данных, нужна наблюдаемость |

### Переключение

```yaml
# Режим памяти
storage:
  backend: "memory"

# LSM-режим
storage:
  backend: "lsm"
  data_dir: ".lsm-data"
```

Оба режима реализуют одинаковые интерфейсы (`Dictionary`, `InvertedIndex`), поэтому Engine работает с ними единообразно.

## Известные ограничения

1. **Однопоточность**: все операции выполняются в одном потоке, блокировки не используются

2. **Нет WAL**: при аварийном завершении данные в memtable теряются. Для полной персистентности необходим Write-Ahead Log

3. **LSMIndex.Add — read-modify-write**: каждый Add читает старый bitmap, объединяет и записывает обратно. При массовой загрузке можно добавить буферизацию в памяти

4. **Bloom filter для composite key**: bloom filter проверяет точный 8-байтовый ключ, но запросы — это prefix scan по featureID. Возможная оптимизация — prefix bloom filter

5. **Нет обновления/удаления документов**: после индексации документ нельзя обновить или удалить из индекса