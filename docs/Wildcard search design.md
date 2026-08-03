# Поиск с подстановочными знаками — Проектная документация

## Обзор

Поверх булевого движка реализованы два вида запросов с символом `*`:

| Тип | Синтаксис | Пример | Описание |
|-----|-----------|--------|----------|
| Префиксный | `prefix*` | `fox*` | Все термы, начинающиеся с префикса |
| K-gram (шаблон) | `*suffix`, `mid*dle`, `h*l*` | `he*o`, `*ound` | Произвольный шаблон с `*` |

Оба типа интегрированы в парсер и могут комбинироваться с `AND`, `OR`, `AND NOT`:

```
fox* AND NOT dog
(he*o OR cat*) AND snake
```

---

## Префиксный поиск

### Идея

Словарь (`Dictionary`) хранит термы в LSM-дереве в лексикографическом порядке.
Префиксный запрос `fox*` естественно отображается на диапазонный скан:

```
tree.Scan("fox", "fox\xff")  →  [fox, foxhound, foxberry, ...]
```

Для каждого найденного терма берётся его `featureID`, после чего posting list-ы объединяются через OR.

### Цепочка вызовов

```
parser.parsePrimary("fox*")
  → engine.SearchPrefix("fox")
      → dict.ScanPrefix("fox")           // диапазонный скан по LSM / перебор map
          → [featureID_fox, featureID_foxhound, ...]
      → для каждого featureID:
          idx.GetPostingList(id)          // bitmap<docID>
      → OR всех bitmap-ов
      → Result
```

### Реализация `ScanPrefix`

**LSMDictionary** — делегирует `tree.Scan`:

```go
func (d *LSMDictionary) ScanPrefix(prefix string) ([]uint32, error) {
    kvs, err := d.tree.Scan(prefix, prefix+"\xff")
    // фильтруем по strings.HasPrefix, декодируем value → featureID
}
```

**MemDictionary** — перебирает map:

```go
func (d *MemDictionary) ScanPrefix(prefix string) ([]uint32, error) {
    for term, id := range d.termToID {
        if strings.HasPrefix(term, prefix) { ... }
    }
}
```

### Важное ограничение

Словарь хранит **обработанные** термы (после стемминга и нормализации).
Запрос `runn*` с включённым стеммингом не найдёт «running», потому что в индексе лежит стем `run`.
Рекомендация: использовать форму основы (`run*`) или отключить стемминг в конфиге.

---

## K-gram поиск

### Идея

Произвольный шаблон (`he*o`, `*ound`) нельзя свести к диапазонному скану.
Классическое решение — **k-gram индекс**: при индексации каждый терм разбивается на
подстроки длины k (биграммы при k=2) с граничными маркерами `$`, и строится отдельный
инвертированный индекс:

```
kgramID  →  bitmap<featureID>
```

Это та же структура, что и основной инвертированный индекс (`featureID → bitmap<docID>`),
поэтому реализация полностью переиспользует `LSMIndex` и `LSMDictionary`.

### Генерация k-gram

Для терма с граничными маркерами:

```
"hello"  →  "$hello$"  →  ["$h", "he", "el", "ll", "lo", "o$"]   (k=2)
```

Функция `Generate(term, k)` в пакете `kgram`.

### Индексация

При вызове `IndexFile` для каждого терма:

```
term "hello", featureID = 5
  → Generate("hello", 2) = ["$h","he","el","ll","lo","o$"]
  → для каждого bigram:
      kgramDict.GetOrAdd(gram)  →  kgramID
  → kgramIdx.Add(featureID=5, kgramIDs=[...])
```

Таким образом, `kgramIdx.GetPostingList(kgramID)` возвращает bitmap всех `featureID`,
чьи термы содержат данный биграм.

### Поиск по шаблону

Пример: `he*o`

**Шаг 1 — извлечение k-gram из шаблона:**

Шаблон разбивается по `*`. Граничные маркеры `$` добавляются только к настоящим
началу и концу паттерна, но не к внутренним сегментам.

```
"he*o"  →  части ["he", "o"]
  i=0 (начало):  "$he"  →  ["$h", "he"]
  i=1 (конец):   "o$"   →  ["o$"]
  итого:  ["$h", "he", "o$"]
```

```
"*ound"  →  части ["", "ound"]
  i=0: пусто, пропускаем
  i=1 (конец):  "ound$"  →  ["ou", "un", "nd", "d$"]
  итого:  ["ou", "un", "nd", "d$"]
```

**Шаг 2 — пересечение posting list-ов:**

```
candidates = postings["$h"] ∩ postings["he"] ∩ postings["o$"]
           = {featureID_hello, featureID_hero}   // могут быть ложные совпадения
```

**Шаг 3 — пост-фильтрация:**

K-gram пересечение может давать ложноположительные результаты.
Каждый кандидат проверяется точным сопоставлением с шаблоном:

```
MatchPattern("hello", "he*o")  →  true  ✓
MatchPattern("hero",  "he*o")  →  true  ✓
```

**Шаг 4 — сборка результата:**

```
для каждого прошедшего featureID:
    idx.GetPostingList(featureID)   →  bitmap<docID>
OR всех bitmap-ов  →  Result
```

### Функция `MatchPattern`

Рекурсивное сопоставление с шаблоном (`*` = любое число символов):

```go
func matchWildcard(s, p []rune) bool {
    if len(p) == 0 { return len(s) == 0 }
    if p[0] == '*' {
        for len(p) > 0 && p[0] == '*' { p = p[1:] }
        for i := 0; i <= len(s); i++ {
            if matchWildcard(s[i:], p) { return true }
        }
        return false
    }
    if len(s) == 0 || s[0] != p[0] { return false }
    return matchWildcard(s[1:], p[1:])
}
```

---

## Архитектура пакета `kgram`

```
kgram/
  kgram.go      ← Generate / ExtractFromPattern / MatchPattern  (чистые функции)
  index.go      ← интерфейс KgramIndex
  mem_impl.go   ← MemKgramIndex   (map + roaring.Bitmap, без LSM)
  lsm_impl.go   ← LSMKgramIndex   (LSMDictionary + LSMIndex)
```

### Интерфейс

```go
type Index interface {
    AddTerm(featureID uint32, term string)
    Search(pattern string) []uint32
}
```

### Хранение `termByID`

Для пост-фильтрации нужно восстановить терм по `featureID`.
Обратный словарь `map[uint32]string` хранится в памяти в обоих реализациях
(и Memory, и LSM). Накладные расходы — порядка 50 байт на терм, для корпуса
из 100 000 термов ≈ 5 МБ.

---

## Маршрутизация в парсере

`parsePrimary` определяет тип токена по наличию `*`:

```
"fox"    → точный поиск  (Search)
"fox*"   → * только в конце → префиксный поиск  (SearchPrefix)
"he*o"   → * внутри / в начале → k-gram поиск   (SearchWildcard)
```

---

## Сравнение двух подходов

| | Префиксный поиск | K-gram поиск |
|--|---|---|
| Поддерживаемые паттерны | только `prefix*` | любые `*` |
| Структура данных | существующий словарь | отдельный k-gram индекс |
| Сложность запроса | O(результатов скана) | O(k-gram × постлисты) |
| Ложные срабатывания | нет | есть, устраняются пост-фильтром |
| Влияние стемминга | прямое | прямое (индексируются стемы) |
| Доп. место на диске | нет | ≈ k × размер словаря |
