# textproc

Конвейер предобработки текста: извлекает термы для инвертированного индекса.
Поддерживает английский и русский языки.

## Схема обработки

```
Исходный текст
  │
  ▼  Tokenizer          Разбиение по границам Unicode-букв и цифр
  │                      "Hello, мир!" → ["Hello", "мир"]
  │
  ▼  Normalizer          Нижний регистр + Unicode NFC + опционально снятие диакритики
  │                      ["Hello", "МИР"] → ["hello", "мир"]
  │
  ▼  StopWordFilter      Удаление стоп-слов (the, is, и, в, …)
  │                      ["the", "quick", "fox"] → ["quick", "fox"]
  │
  ▼  Stemmer             Стемминг Snowball
  │                      ["running", "dogs"] → ["run", "dog"]
  │
  ▼  Дедупликация
  │
  Результат: Lang + Terms + Tokens
```

Язык определяется автоматически по распределению символов в тексте.

## Быстрый старт

```go
import "omolsm/internal/invertedindex/textproc"

// Конфигурация по умолчанию, язык определяется автоматически
p := textproc.NewPipeline()
result := p.Process("The quick brown foxes are jumping over the lazy dogs")

result.Lang   // "english"
result.Terms  // ["quick", "brown", "fox", "jump", "lazi", "dog"]
result.Tokens // полный поток токенов (с позициями, без дедупликации)
```

```go
// Русский текст
result := p.Process("Быстрая коричневая лиса прыгает через ленивую собаку")

result.Lang   // "russian"
result.Terms  // ["быстр", "коричнев", "лис", "прыга", "ленив", "собак"]
```

## Конфигурация

Любой этап можно заменить или отключить через functional options:

```go
// Жёстко задать язык
p := textproc.NewPipeline(textproc.WithLanguage(textproc.LangRussian))

// Отключить фильтрацию стоп-слов (сценарий обработки запроса)
p := textproc.NewPipeline(textproc.WithStopWordFilter(nil))

// Отключить стемминг
p := textproc.NewPipeline(textproc.WithStemmer(nil))

// Включить снятие диакритики (café → cafe)
p := textproc.NewPipeline(
    textproc.WithNormalizer(textproc.NewUnicodeNormalizer(textproc.WithStripAccents(true))),
)

// Свой токенизатор
p := textproc.NewPipeline(textproc.WithTokenizer(myTokenizer))
```

## Структура пакета

```
textproc/
├── pipeline.go       Оркестратор Pipeline + functional options
├── tokenizer.go      Интерфейс Tokenizer + UnicodeTokenizer
├── normalizer.go     Интерфейс Normalizer + UnicodeNormalizer (NFC + регистр + диакритика)
├── stopwords.go      Интерфейс StopWordFilter + встроенные списки стоп-слов
├── stemmer.go        Интерфейс Stemmer + адаптер SnowballStemmer
├── lang.go           Константы Lang + определение языка
└── pipeline_test.go  Тесты
```

## Компоненты

### Tokenizer

Работает на базе `strings.FieldsFunc`: разделителями считаются все небуквенные и
нецифровые символы. `unicode.IsLetter` поддерживает латиницу и кириллицу без
дополнительной обработки.

### Normalizer

Три шага: Unicode-нормализация NFC → приведение к нижнему регистру → опциональное
снятие диакритики. Диакритика снимается разложением строки в форму NFD, удалением
всех комбинирующих знаков (`unicode.Mn`) и обратной сборкой в NFC.

### StopWordFilter

Структура данных — `map[Lang]map[string]struct{}`: первый уровень разделяет языки,
второй хранит множество стоп-слов языка. Проверка — поиск по map за O(1). Встроено
около 120 английских и около 150 русских стоп-слов, список расширяется через
`AddCustomWords`.

### Stemmer

Обёртка над `github.com/kljensen/snowball`, поддерживает английский и русский.
Для `LangUnknown` возвращает слово без изменений.

### Определение языка

`DetectLang` считает количество кириллических и латинских букв в тексте:

- обе доли выше 20 % → `LangMixed`;
- доля кириллицы выше 50 % → `LangRussian`;
- иначе → `LangEnglish`;
- букв нет вовсе → `LangUnknown`.

Для смешанных документов язык каждого токена определяется отдельно через
`DetectTokenLang` — по наличию кириллических символов в самом токене.

## Индексация против запроса

Для индексации и для разбора запроса используются разные конфигурации конвейера:

```go
// Индексация: полный конвейер
indexPipeline := textproc.NewPipeline()

// Запрос: без удаления стоп-слов (пользователь может искать любое слово)
queryPipeline := textproc.NewPipeline(textproc.WithStopWordFilter(nil))
```

Важно: `stemming`, `normalize` и `strip_accents` должны совпадать в обеих
конфигурациях — иначе термы запроса не совпадут с термами индекса. Конфигурация
движка проверяет это при загрузке.

## Зависимости

```bash
go get github.com/kljensen/snowball
go get golang.org/x/text
```

## Тесты

```bash
go test ./internal/invertedindex/textproc/ -v
```
