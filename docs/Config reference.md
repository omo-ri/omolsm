# Справочник конфигурации

## Обзор

Система использует два уровня конфигурации:

```
engine.Config (YAML)           config.Config (Go)
┌─────────────────────┐        ┌──────────────────────────┐
│ source.dir           │        │ Dir                       │
│ language             │        │ MaxLevel                  │
│ storage.backend      │        │ SSTSize                   │
│ storage.data_dir     │        │ SSTNumPerLevel            │
│ storage.sst_size ────┼──map──→│ SSTDataBlockSize          │
│ storage.sst_block_size│       │ SSTFooterSize             │
│ storage.max_level    │        │ Filter                    │
│ index.*              │        │ FilterConstructor         │
│ query.*              │        │ MemTableConstructor       │
│                      │        │ MergeIteratorFactory      │
│                      │        │ Hooks                     │
└─────────────────────┘        └──────────────────────────┘

engine.Config — конфигурация поискового движка (из YAML)
config.Config — конфигурация LSM Tree (программная)
```

Преобразование: `StorageConfig.ToLSMConfigOptions() → []config.ConfigOption`

## engine.Config (YAML)

### source

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `dir` | string | `"./documents"` | Директория с файлами для индексации |
| `encoding` | string | `"utf-8"` | Кодировка файлов |
| `extensions` | []string | `[".txt"]` | Расширения файлов для индексации |

### language

| Значение | Описание |
|---------|---------|
| `"auto"` | Автоопределение по содержимому (по умолчанию) |
| `"english"` | Английский: English stemmer + English stop words |
| `"russian"` | Русский: Russian stemmer + Russian stop words |
| `"mixed"` | Смешанный: оба стеммера, оба набора стоп-слов |

### storage

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `backend` | string | `"memory"` | `"memory"` или `"lsm"` |
| `data_dir` | string | `".lsm-data"` | Директория для SST-файлов (только для LSM) |
| `block_size` | int | `1024` | Размер блока docID для инвертированного индекса |
| `sst_size` | int | `4194304` | Порог сброса memtable в байтах (4 MB) |
| `sst_num_per_level` | int | `10` | Количество SST на уровне до compaction |
| `sst_block_size` | int | `4096` | Размер data block в SST (4 KB) |
| `max_level` | int | `4` | Максимальное количество уровней LSM |

### index / query

| Поле | Тип | По умолчанию (index) | По умолчанию (query) | Описание |
|------|-----|---------------------|---------------------|----------|
| `stop_words` | bool | `true` | `false` | Удалять стоп-слова |
| `stemming` | bool | `true` | `true` | Применять стемминг |
| `strip_accents` | bool | `false` | `false` | Удалять диакритические знаки |
| `normalize` | bool | `true` | `true` | Нормализация (lowercase и т.д.) |
| `min_term_length` | int | `1` | `1` | Минимальная длина терма |

**Ограничение**: `stemming`, `strip_accents` и `normalize` должны совпадать между `index` и `query`. Валидация вернёт ошибку при несовпадении.

### custom_stop_words

```yaml
custom_stop_words:
  english:
    - "foo"
    - "bar"
  russian:
    - "тест"
```

Добавляются к стандартным спискам стоп-слов.

## config.Config (Go)

### Программные параметры

Эти параметры недоступны через YAML и задаются только в коде:

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|-------------|----------|
| `Filter` | `filter.Filter` | `BloomFilter(10240)` | Экземпляр фильтра для чтения (read path) |
| `FilterConstructor` | `func() Filter` | `BloomFilter(10240)` | Фабрика фильтров для записи (write path) |
| `MemTableConstructor` | `func() MemTable` | `TreeMapMemTable` | Фабрика memtable |
| `MergeIteratorFactory` | `MergeIteratorFactory` | `nil` (KV-режим) | Стратегия слияния при compaction |
| `Hooks` | `Hooks` | `{}` | Callback-и событий |
| `SSTFooterSize` | `int` | `32` | Размер footer в SST (фиксированный) |

### ConfigOption

```go
config.WithMaxLevel(7)
config.WithSSTSize(8 * 1024 * 1024)
config.WithSSTDataBlockSize(8 * 1024)
config.WithSSTNumPerLevel(5)
config.WithFilter(myFilter)
config.WithFilterConstructor(func() filter.Filter { ... })
config.WithMemtableConstructor(memtable.NewMapMemTable)
config.WithMergeIteratorFactory(iterator.BitmapMergeFactory())
config.WithHooks(config.Hooks{ ... })
```

## Hooks

### FlushInfo

```go
type FlushInfo struct {
    Level   int      // целевой уровень (обычно 0)
    SSTSize uint64   // размер созданного SST
    KVCount int      // количество KV-пар
    MemSize int      // размер memtable до flush (байт)
}
```

### CompactionInfo

```go
type CompactionInfo struct {
    FromLevel  int      // исходный уровень
    ToLevel    int      // целевой уровень
    InputFiles int      // количество входных SST
    OutputSize uint64   // размер выходного SST
}
```

### Пример

```go
conf, _ := config.NewConfig(dir,
    config.WithHooks(config.Hooks{
        OnFlush: func(info config.FlushInfo) {
            log.Printf("[FLUSH] L%d: %d KV, %d байт SST, memtable был %d байт",
                info.Level, info.KVCount, info.SSTSize, info.MemSize)
        },
        OnCompaction: func(info config.CompactionInfo) {
            log.Printf("[COMPACT] L%d → L%d: %d файлов → %d байт",
                info.FromLevel, info.ToLevel, info.InputFiles, info.OutputSize)
        },
        OnWrite: func(key []byte, valueSize int) {
            // Внимание: вызывается на каждый Put, может быть накладно
        },
    }),
)
```

## Примеры YAML

### Минимальный (in-memory)

```yaml
source:
  dir: "./documents"
language: "auto"
storage:
  backend: "memory"
```

### LSM с настройками по умолчанию

```yaml
source:
  dir: "./documents"
language: "auto"
storage:
  backend: "lsm"
```

### LSM с тонкой настройкой

```yaml
source:
  dir: "./corpus"
  extensions: [".txt", ".md"]
language: "english"
storage:
  backend: "lsm"
  data_dir: ".index-data"
  block_size: 2048
  sst_size: 8388608          # 8 MB
  sst_num_per_level: 5
  sst_block_size: 8192       # 8 KB
  max_level: 5
index:
  stop_words: true
  stemming: true
  strip_accents: true
  normalize: true
  min_term_length: 2
query:
  stop_words: false
  stemming: true
  strip_accents: true
  normalize: true
  min_term_length: 2
custom_stop_words:
  english: ["http", "https", "www"]
```

### Русскоязычный корпус

```yaml
source:
  dir: "./документы"
  encoding: "utf-8"
  extensions: [".txt"]
language: "russian"
storage:
  backend: "lsm"
  data_dir: ".lsm-данные"
  sst_size: 4194304
  sst_num_per_level: 10
index:
  stop_words: true
  stemming: true
  normalize: true
query:
  stop_words: false
  stemming: true
  normalize: true
custom_stop_words:
  russian: ["также", "однако", "поэтому"]
```

## Рекомендации по настройке

### Маленький корпус (< 1000 документов)

Используйте `memory`. LSM не даёт преимуществ при малом объёме данных.

### Средний корпус (1K–100K документов)

```yaml
storage:
  backend: "lsm"
  sst_size: 4194304        # 4 MB
  sst_num_per_level: 10
  max_level: 4
```

### Большой корпус (> 100K документов)

```yaml
storage:
  backend: "lsm"
  sst_size: 16777216       # 16 MB — реже flush
  sst_num_per_level: 10
  sst_block_size: 16384    # 16 KB — больше данных за одно чтение
  max_level: 6             # больше уровней
```

### Диагностика проблем

| Проблема | Возможная причина | Решение |
|----------|------------------|---------|
| Медленная индексация | Слишком частый flush | Увеличить `sst_size` |
| Много SST-файлов | Редкий compaction | Уменьшить `sst_num_per_level` |
| Высокое потребление RAM | Большой memtable | Уменьшить `sst_size` |
| Медленные запросы | Много SST на уровне | Уменьшить `sst_num_per_level` |
| Поиск не находит слова | Несовпадение stemming | Проверить `index.stemming == query.stemming` |