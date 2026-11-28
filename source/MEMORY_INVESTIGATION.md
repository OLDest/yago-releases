# Память: расследование 0.6.1

## Подтверждённые причины и исправления

1. В стандартной библиотеке Go 1.27.4 есть статический `crypto/internal/fips140/drbg.memory` размером 33 554 432 байта. Он находится вне Go heap. В нативном тесте даже пустое окно сопровождалось переходом почти всего этого блока в resident memory. Какой именно системный компонент трогает страницы, не установлено. Проверка с Go 1.26.11 показала тот же блок; смена версии компилятора сама по себе не помогла.
2. Релизная сборка содержит воспроизводимую правку локального Go: `sync.OnceValue` выделяет тот же буфер только при первом вызове FIPS getEntropy. Изменён только способ выделения памяти. Исходный и исправленный файлы проверяются SHA-256. Полный исправленный файл, лицензия и скрипт включены в архив исходников. Статический zero-fill BSS Windows EXE уменьшился примерно с 32,4 MiB до 375 352 байт.
3. Ресемплер 48 000 → 44 100 Гц создавал массивы на каждую стереопару. Буферы теперь хранятся в ресемплере. Это влияет на треки с отличающейся частотой, не на каждый MP3.
4. Чтение закешированных байтов создавало context.AfterFunc на каждый короткий Read. Теперь callback нужен только при ожидании ещё не загруженных данных. Поток буферизуется по 32 КиБ.
5. go-mp3 выделял целиком ведущий ID3 tag, чтобы пропустить его. Новый источник пропускает tag и встроенную картинку через ограниченный буфер, сохраняя относительную перемотку.
6. API копировал весь JSON-ответ, затем отдельно result. Для обычных endpoint result теперь разбирается прямо в структуру назначения. Для маленьких batch «Моей волны» сохранена поддержка прямого и обёрнутого формата.
7. Переход к новой библиотеке отменяет предыдущий запрос и его пакетную загрузку. Выход, вход и деморежим также отменяют старую загрузку.

## Сопоставимый нативный тест

Windows, Gio, окно 1080×740, демобиблиотека без аккаунта и звука, примерно 30 секунд. Go 1.27.4 до/после lazy allocation. Тестовый процесс отличается от обычного EXE и использует диагностический GC; принудительного GC или обрезки Working Set в приложении нет.

| Показатель | До | После |
|---|---:|---:|
| Working Set после диагностического GC | 108 318 720 | 70 074 368 |
| Private Bytes | 97 284 096 | 62 521 344 |
| Resident страницы zero-fill image | 33 943 552 | 339 968 |

В обоих запусках heap после прогрева стабилен: примерно 6 MiB, число goroutine 11. Это не доказательство отсутствия всех возможных утечек: длительное прослушивание под аккаунтом отдельно не измерено.

## Выделения в одинаковых микротестах

| Нагрузка | До | После |
|---|---:|---:|
| 1 секунда входного PCM 48 кГц | 385 427 B / 92 105 alloc | 16 882 B / 4 alloc |
| 65 536 байт дискового кеша, Read по 4 байта | 2 361 322 B / 49 157 alloc | 28 B / 0 alloc |
| API-плейлист из 5 000 синтетических треков | 7 775 204 B | 6 380 268 B |
| Пропуск ID3 на 8 MiB новым источником | — | около 33 KiB |

Это общий объём аллокаций за операцию, а не постоянно занятая RAM. Небольшое ненулевое B/op при нуле alloc в дисковом benchmark — амортизированная служебная работа.

30 быстрых смен трека/seek/stop с настоящим MP3: Go heap 962 128 → 1 126 808 байт, goroutine 7 → 7, каталог временных аудиофайлов пуст. Проверены пауза, обычная и относительная перемотка, восстановление позиции, декодирование MP3 с ID3 и перемотка после EOF.

## Проверки криптографии

На исправленной локальной стандартной библиотеке прошли crypto/rand и crypto/ed25519, включая отдельный запуск с GODEBUG=fips140=on. Прошёл crypto/tls с -short, исключая TestBogoSuite, и тестовым tlssecpmlkem=1 для ожидаемых самим TLS-набором новых defaults. Это значение выставлялось только в тестовом процессе: приложение сохраняет прежние defaults своего go.mod.

Полный внешний TLS-набор не считается пройденным: BoGo упирался в запрет IPv6 loopback, RealResumption — в сбросы соединения внешним сервером; исходный запуск также выявил несовпадение defaults Go 1.25 в go.mod и ожиданий тестов Go 1.27. Результаты не скрыты. Нативные тесты приложения, go vet, APK-подпись/ABI/ELF и живое подписанное обновление проверяются отдельно.

## Как повторить

`prepare-toolchain.ps1` применяет проверенный по SHA-256 файл из `toolchain-patches/` только к Go 1.27.4. `go.ps1` и Android environment выполняют его автоматически. CI фиксирует ту же версию и выполняет тот же скрипт. Системный Go не менялся.

Для нативного профиля задайте YAGO_NATIVE_MEMORY_DIR и запустите TestNativeMemoryAudit; YAGO_NATIVE_MEMORY_MODE=blank включает пустое окно. Тест сам закрывает своё окно. Профили содержат только синтетические данные из этого запуска и остаются локально. Для реального MP3 задайте YAGO_TEST_MP3.

Проверка EXE: `node scripts/check-bss.cjs YaGo.exe`. Проверка аллокаций: benchmarks Resampler48000, CompletedSpoolSmallReads, SkipLargeID3 и LargePlaylistResponse.

Фиксированные «32 МБ всего» не обещаются. В обычных EXE отдельные наблюдения Working Set всё ещё различались и не образуют строгой пары: Windows меняет residency страниц; нативная графика и размер окна также влияют на результат. Подтверждённое изменение — удаление статического блока 32 MiB и уменьшение перечисленных аллокаций.
# Live-session investigation: release 0.9.0

An isolated copy of the release was launched with its own state directory. Before
music, Windows reported working set 68,239,360 bytes (65.1 MiB), private memory
63,700,992 bytes (60.8 MiB). Its periodic metrics reported Go heap 7,099,320 bytes
(6.8 MiB). This reproduces the high total without playing or buffering a song.

Actual playback was attempted, but position remained zero. A separate audio
probe established the exact cause in this session: WASAPI device not found;
WinMM waveOutOpen failed with "No more data is available". Desktop screenshot
capture also failed. This is **not** a completed audible playback or authenticated
Wave test. No conclusion about a long-running Wave session follows from it.

Matched 1080x740 native tests isolated startup costs (30-second after-GC samples;
all three use the same diagnostic profiler, so compare these with each other,
not directly with the smaller release process):

| Test | Working set | Private memory | Live Go heap |
| --- | ---: | ---: | ---: |
| Native window, no Gio renderer | 26.9 MiB | 22.5 MiB | 1.1 MiB |
| Same window, blank Gio rendering | 52.2 MiB | 46.5 MiB | 1.1 MiB |
| Full YaGo demo interface, no music | 72.0 MiB | 64.7 MiB | 6.1 MiB |

Gio rendering adds about 25 MiB in this environment even for a blank surface.
The full interface adds fonts, glyph/vertex caches and application services.
The heap profile identifies vertex cache (~1.1 MiB), system font metadata
(~1.5 MiB) and font parsing/glyph data among retained allocations. Loaded modules
include d3d11.dll and dxgi.dll; the precise graphics allocation/driver breakdown
was not measured. Do not assign all remaining private bytes to a specific driver.
Full UI heap after diagnostic GC decreases from 6,435,304 at 10 seconds to
6,378,840 at 30 seconds. The old 32 MiB static buffer is still absent: diagnostic
image zero-fill is only 392,112 bytes.

A separate **decoder-only** test ran the real network spool, MP3 decoder and
resampler at listening speed for 93.99 seconds: three track opens, 15,876,000 PCM
bytes, no artificial GC during decoding. Go heap samples stay approximately
1–2.3 MiB; final retained heap is 823,472 versus initial 675,944 bytes (+147,528).
Worker count returns to three and no audio spool files remain. This checks the
decoder path, not OS audio output, UI playback or Wave buffering.

Raw evidence is preserved under `.tools/memory-playback/{blank,window-only,full-ui}`
and `internal/audio/.tools/memory-playback/decode/samples.jsonl`. The failed first
playback samples in `.tools/memory-playback/live` must not be used as proof of
advancing audio.

Fixed an independently confirmed defect: Oto closes its ready channel even if
driver initialization fails. YaGo now checks `Context.Err()` after readiness and
while playing, and displays the actual failure rather than claiming playback at
zero seconds. An opt-in regression test reproduced the unavailable device and
verified error propagation. Normal `go test ./...` and `go vet ./...` pass.

Conclusion: these measurements attribute a large startup rise to window/rendering
and UI initialization. They do not establish that the user's full 25-to-67 MiB
trajectory has the same timing, nor rule out longer playback/account-specific
leaks. Reaching a sustained 25 MiB with the current renderer is not demonstrated.
Next required measurement is sustained real Wave playback with a usable audio
endpoint. No working-set trimming, GC loop or memory-limit reduction was added.

## Font and graphics cleanup after 0.9.0

- Removed Go Medium: all actual labels/buttons use Regular; only the brand uses
  Bold. Corrected inferred Go Bold weight from SemiBold to the requested Bold.
- Replaced eager Material theme construction with a minimal theme. Unused radio
  icons are absent; checkbox icons are constructed once when update settings open.
- The ordinary UI shaper uses bundled fonts only. Every dynamic text label,
  button and editor checks font coverage before shaping. The first missing glyph
  enables system fallback once and replaces the existing shaper; there is no
  second persistent cache or unbounded per-string coverage cache. Latin/Cyrillic
  titles remain on bundled faces; other scripts still use installed system fonts.
- The selected-row play triangle is now a vector, and offline target selection
  uses `[x]`. Those two controls no longer load a whole fallback font just for
  `▶` or `✓`. UI glyph coverage is checked in both bundled faces.
- The 32x32 brand image is already one reused 4096-byte RGBA asset. The large icon
  master is not loaded by the app. Remaining renderer/path/glyph caches are used.
  Direct3D and cross-platform/fallback rendering support were retained.

Matched 30-second native profiles: retained Go heap after diagnostic GC changes
from 6,378,840 to 4,374,944 bytes (about 1.91 MiB / 31% less). Working set changes
from 75,538,432 to 72,388,608 bytes in that pair, but separate unprofiled runs vary
substantially: one pair regressed and the repeat improved by about 3 MiB. A fixed
total process-memory improvement is therefore **not established** by these short
runs. OS/graphics memory must not be confused with retained Go heap.

Stripped Windows EXE with identical build options: 13,320,704 -> 13,168,640 bytes
(-152,064 bytes, 148.5 KiB). No dependencies were added. Raw profiles are in
`.tools/memory-playback/font-final-marker`, comparisons in `font-comparison`.
At measurement time the Windows candidate was
`.tools/memory-playback/YaGo-diagnostic.exe`. The cleanup is included in the
0.9.1 Windows/Android release; publication verification is recorded in PLAN.md.

Validation: `go test ./...`, `go vet ./...`; font coverage and lazy fallback tests;
24 rendered UI layouts from 320x480 to 1600x829; ordinary layout paths assert that
they do not enable system fonts. Windows executable build and BSS guard pass.
