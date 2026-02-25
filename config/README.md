# Config

Пакет конфигурации для OmoLSM. Использует паттерн функциональных опций (Functional Options) для гибкой настройки параметров.

## Быстрый старт

```go
// Только указать директорию — остальное по умолчанию
cfg, err := config.NewConfig("./data")

// Кастомная конфигурация
cfg, err := config.NewConfig("./data",
    config.WithMaxLevel(7),
    config.WithSSTSize(1*1024*1024),
    config.WithSSTDataBlockSize(16*1024),
    config.WithSSTNumPerLevel(20),
)
```

Если указанная директория не существует, она будет создана автоматически.

## Параметры

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `Dir` | — (обязательный) | Директория для хранения SST-файлов |
| `MaxLevel` | 4 | Количество уровней LSM-Tree |
| `SSTSize` | 4 MB | Максимальный размер одного SSTable (также порог сброса MemTable на диск) |
| `SSTDataBlockSize` | 4 KB | Размер Data Block внутри SSTable |
| `SSTNumPerLevel` | 10 | Максимальное количество SSTable-файлов на каждом уровне |
| `SSTFooterSize` | 32 B | Фиксированный размер Footer в SSTable (не настраивается) |
| `Filter` | Bloom Filter (10240 бит) | Экземпляр фильтра для пути чтения (без состояния, общий) |
| `FilterConstructor` | Конструктор Bloom Filter | Фабрика фильтров для пути записи — создаёт отдельный экземпляр для каждого SST Writer |
| `MemTableConstructor` | TreeMap | Реализация MemTable |

## Доступные опции

```go
// Количество уровней LSM-Tree
config.WithMaxLevel(7)

// Размер SSTable (в байтах)
config.WithSSTSize(1 * 1024 * 1024)

// Размер Data Block (в байтах)
config.WithSSTDataBlockSize(16 * 1024)

// Количество SSTable на уровень
config.WithSSTNumPerLevel(20)

// Пользовательский фильтр (путь чтения)
config.WithFilter(myFilter)

// Фабрика фильтров (путь записи)
config.WithFilterConstructor(func() filter.Filter {
    return filter.NewBloomFilter(20480)
})

// Пользовательская реализация MemTable
config.WithMemtableConstructor(memtable.NewSkipListMemTable)
```

## Пример: полная конфигурация

```go
cfg, err := config.NewConfig("./data",
    config.WithMaxLevel(5),
    config.WithSSTSize(2*1024*1024),
    config.WithSSTDataBlockSize(8*1024),
    config.WithSSTNumPerLevel(15),
    config.WithFilter(filter.NewBloomFilter(20480)),
    config.WithFilterConstructor(func() filter.Filter {
        return filter.NewBloomFilter(20480)
    }),
    config.WithMemtableConstructor(memtable.NewSkipListMemTable),
)
if err != nil {
    log.Fatal(err)
}
```