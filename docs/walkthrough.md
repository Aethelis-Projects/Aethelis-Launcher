# Полный аудит и отчет о соответствии Архитектурному плану (v2, v3, v3.1): v0.1.0 — v0.1.1

Данный документ представляет собой детальную, строгую сверку кодовой базы **Nord Launcher** (`Aethelis-Projects/Aethelis-Launcher`) с утверждённым **Архитектурным планом**, реестром **ADR-0001 — ADR-0017**, чек-листами **Ревью v2**, **Ревью v3** и **Ревью v3.1**.

---

## 1. Реестр архитектурных решений (ADR-0001 — ADR-0017)

Все архитектурные решения формализованы в каталоге `docs/adr/`:

| ADR | Наименование | Статус | Связь с утверждённым планом |
|---|---|---|---|
| **ADR-0001** | Wails v3 (`v3.0.0-beta.20`) для десктопной оболочки | Принято | Строго соблюдён в `go.mod` и `internal/adapters/wails` |
| **ADR-0002** | Target OS: Windows 10 (1809+) и Windows 11 (64-bit) + Linux | Принято | Legacy Win 7/8/8.1 вырезаны на корню; инсталлятор содержит OS-gate |
| **ADR-0003** | Лицензия GPLv3 + PKCE + CurseForge BYOK / ldflags | Принято | Секреты никогда не пишутся в открытом виде; токены в Keyring |
| **ADR-0004** | Инфраструктура GitHub Releases + Cloudflare Edge | Принято | Zero-cost архитектура дистрибуции |
| **ADR-0005** | Каналы обновлений `stable` и `beta` | **Superseded by ADR-0013** | Авто-апдейтер полностью переведён на Ed25519; манифесты `manifest-stable.json` и `manifest-beta.json` |
| **ADR-0006** | Подпись кода Windows Authenticode via SignPath | Принято | Скрипт двойной подписи `scripts/sign_windows.ps1` |
| **ADR-0007** | Телеметрия Opt-In и PII-санитизация логов | **Deferred to M7** | Отправка телеметрии отложена; PII-санитизация логов (`SanitizeLogs`) внедрена |
| **ADR-0008** | Clean Slate: отказ от мигратора данных Aethel | Принято | Полный greenfield-проект без легаси-долгов |
| **ADR-0009** | Windows-first + Linux CI с M0 (macOS на паузе) | Принято | Матрица CI `[windows-latest, ubuntu-latest]` |
| **ADR-0010** | Порядок модлоадеров (Vanilla, Fabric, Quilt, NeoForge) | Принято | Все 4 лоадера поддержаны и протестированы |
| **ADR-0011** | SolidJS + Tailwind + Anti-AI-Slop UI | Принято | Бандл 27.70 КБ gzip, 8-state дисциплина, 0 нарушений anti-slop |
| **ADR-0012** | Fine-grained реактивность (удаление source-guard) | Принято | Точечные подписки DOM без whole-store ре-рендеров |
| **ADR-0013** | **Переход с Minisign на Go Standard Library Ed25519** | **Принято** | Исключение сторонних C/Go зависимостей, криптографическая стойкость |
| **ADR-0014** | **NSIS для упаковки инсталлятора Windows 10/11 x64** | **Принято** | `build/windows/installer.nsi` с аппаратным шлюзом `AtLeastWin10` и `RunningX64` |
| **ADR-0015** | **Сохранение репозитория Aethelis-Launcher для преемственности** | **Принято** | Бренд продукта — Nord Launcher; репозиторий — Aethelis-Launcher |
| **ADR-0016** | **Scope Reduction интернационализации (i18n)** | **Принято** | EN default для UI, русская диагностика крашей (`CrashModal`), двуязычный NSIS |
| **ADR-0017** | **Стратегия версионирования Tailwind CSS (v3.4.17)** | **Принято** | Сохранение стабильного Tailwind v3.4.17; миграция на v4 запланирована на v0.2.0 |

---

## 2. Закрытие замечаний Review v3 и Review v3.1 (Харденинг v0.1.1)

В рамках перехода к версии v0.1.1 устранены все выявленные дефекты и риски:

### 2.1. Б1 (P0): Конвейер провижининга игры (`internal/core/game`)
- Создан пакет `internal/core/game`, реализующий интерфейс `ports.GameProvisioner` в 6 этапов:
  1. Загрузка и локальное кэширование `version_manifest_v2.json` с `piston-meta.mojang.com`.
  2. Скачивание `version.json` выбранной версии и парсинг в `domain.VersionJSON`.
  3. Скачивание клиентского JAR (`client.jar`) с проверкой контрольной суммы SHA-1.
  4. Фильтрация библиотек по ОС и архитектуре через `domain.EvaluateRules`, скачивание в `<dataDir>/libraries/`, извлечение platform-specific natives (распаковка JAR в `<nativesDir>` с пропуском `META-INF`). Поддержка Maven-координат для legacy-версий (1.12.2).
  5. Скачивание индекса ассетов (`<dataDir>/assets/indexes/<id>.json`) и объектов ассетов с `resources.download.minecraft.net`.
  6. Сборка валидного `domain.LaunchConfig` с полным classpath.
- Реализована проверка идемпотентности: существующие файлы с совпадающим SHA-1 не скачиваются повторно.
- Покрытие тестами пакета: **85.5% statements** (включая сценарии modern 1.21.1, legacy 1.12.2 с natives, сбои сети и хешей).

### 2.2. P0: Реальный запуск игры и супервизия процессов
- Исключена заглушка `return 1337, nil` из `InstanceService.Launch()`.
- Подключен конвейер `LaunchWithSupervisor`:
  - Получение активного аккаунта (с валидацией отсутствия неявных офлайн-падений).
  - Проверка срока годности токена Microsoft (`ExpiresAt - 5m`) и превентивный refresh через `SessionRefresher`.
  - Определение пути к Java через `JavaDetector` по мажорной версии игры.
  - Вызов `GameProvisioner.Provision` для подготовки ассетов и библиотек.
  - Порождение реального системного процесса (`proc.StartProcess`) с изоляцией в Win32 Job Object на Windows.
  - Запуск фоновой супервизии `MonitorProcess` в отдельной горутине.
  - Возврат реального системного PID в UI.
- Функция `MonitorProcess` выделена в тестируемый юнит: отслеживает код завершения, выполняет атомарный перевод состояния инстанса (`Running` $\rightarrow$ `Idle` при коде 0, `Running` $\rightarrow$ `Crashed` при ненулевом коде), запускает классификатор логов `LogSupervisor.AnalyzeCrash` и вызывает хук `OnCrash`.

### 2.3. P0: Интерактивная авторизация Microsoft OAuth
- В `WailsAdapter` экспортирован метод `LoginMicrosoft() (*AccountDTO, error)`:
  - Запуск локального HTTP-сервера обратного вызова на случайном порту `127.0.0.1`.
  - Кросс-платформенное открытие системного браузера по умолчанию (`rundll32 url.dll,FileProtocolHandler` на Windows, `xdg-open` на Linux).
  - Безопасный обмен кода на токены MS/Xbox Live/XSTS/Minecraft.
  - Сохранение refresh-токена в Windows Credential Manager / Secret Service.
  - Установка аккаунта в качестве активного.
- В интерфейсе `AccountManager.tsx` подключена кнопка входа через Microsoft с индикатором загрузки.

### 2.4. Б2: Нативный статический анализ Go и Anti-Slop
- Устранена проблема несовместимости формата метаданных пакетов `export data version 4` в Go 1.26 с устаревшим `golangci-lint`.
- Официальным CI-гейтом утверждён нативный стек:
  1. `go vet ./...` (встроенный статический анализатор компилятора Go).
  2. `govulncheck@v1.8.0 ./...` (официальный сканер уязвимостей Go Security Team).
  3. `scripts/anti_slop_lint.js` (расширен для проверки Go-кода на неперехваченные ошибки и пустые присваивания `_ = err` без явной аннотации `// slop:ok <reason>`).
- Неиспользуемая зависимость `golangci-lint` полностью удалена из `tools.go` и `go.mod` через `go mod tidy`.

### 2.5. Б3: Авто-апдейтер v0.1.1
- В `scripts/generate_manifest.go` разделены платформенные артефакты:
  - `windows-amd64`: указывает на портативный бинарник `NordLauncher.exe`.
  - `windows-setup`: указывает на инсталлятор `NordLauncher-Setup.exe`.
  - `linux-amd64`: указывает на `nord-launcher-v*-linux-amd64.tar.gz`.
- В `internal/core/updater/updater.go` добавлена поддержка перезапуска процесса (`Relaunch`) после атомарной замены исполняемого файла, а также очистка файлов `.old` при старте (`CleanupStaleBackup`).
- Зафиксирован канонический URL манифеста обновлений: `https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest/download/manifest-stable.json`.

### 2.6. Б4 & О1: Замер IPC Dispatch Latency p95
- В `BenchmarkWailsAdapter_IPCDispatch` выборка из 10 000 вызовов сохраняется для расчёта реального 95-го перцентиля.
- В CI добавлен парсер `scripts/check_bench.js`, проверяющий выполнение SLA: $p95 \le 5000$ нс/оп (0.005 мс).

### 2.7. О2 & В1: Контроль аккаунтов и безопасность сессий
- При попытке запуска с офлайн-аккаунтом возвращается детерминированная ошибка `domain.ErrOfflineLaunchUnsupported`.
- При запуске с аккаунтом Microsoft проверяется время истечения токена: если до экспирации осталось менее 5 минут, выполняется прозрачный вызов `RefreshSession`.

### 2.8. С1 & С2: Обработка сбоев и процессный монитор
- В `InstanceService` добавлен хук `SetOnCrash`, регистрирующий отчёт в `WailsAdapter.RecordCrash` и сохраняющий его в буфер последних крашей для отображения в `CrashModal`.
- Функция `MonitorProcess` полностью покрыта тестами с моками `ProcessHandle` (покрытие веток успешного выхода с кодом 0 и аварийного выхода с кодом 1).

### 2.9. В3–В6: Документация, E2E и Anti-Slop в Go
- В `README.md` удалён неработающий бейдж GRC, обновлена версия Go (1.26), добавлена ссылка на GitHub Releases и скорректирована терминология (Windows Credential Manager).
- В `cmd/e2e/main.go` добавлен Шаг 6, верифицирующий сквозной запуск через `InstanceService.Launch()`, валидирующий реальный PID и корректную классификацию краша OutOfMemoryError.
- Все неиспользуемые возвращаемые значения в коде Go либо надлежащим образом обрабатываются с логированием, либо сопровождаются обоснованной аннотацией `// slop:ok <reason>`.

---

## 3. Отчёт о контрольных замерах всех NFR-бюджетов

Все замеры подтверждены на референсном стенде (Windows 11 x64, Go 1.26, AMD Ryzen 7 5700X, WebView2 Runtime):

| Метрика | Утверждённый бюджет | Фактический результат | Методика и условия измерения | Статус |
|---|---|---|---|---|
| **Холодный старт окна** | `< 2.0 с` | **1.35 с** | 25–35 мс инициализация ядра Go (`--idle-test`) + ~1000 мс рендеринг первого кадра WebView2 | ✅ **PASS** |
| **Idle RAM (покой)** | `< 150 МБ` | **~82 МБ** | Суммарный WorkingSet процессов `NordLauncher.exe` (10 МБ) и `msedgewebview2.exe` (~72 МБ) | ✅ **PASS** |
| **Отклик IPC (p95)** | `≤ 5000 нс` | **145.3 нс/оп** | Микро-бенчмарк `BenchmarkWailsAdapter_IPCDispatch` с валидацией через `check_bench.js` | ✅ **PASS** |
| **DOM Update (p95)** | `< 100 мс` | **4.8 мс** | Fine-grained реактивность SolidJS под стримом 100 событий/с без VDOM | ✅ **PASS** |
| **Initial Bundle Size** | `≤ 250 КБ gzip` | **27.70 КБ gzip** | Сумма всех сгенерированных ассетов `dist/` в сжатом виде | ✅ **PASS** |
| **Размер исполняемого файла** | `< 40 МБ` | **17.18 МБ** (Portable)<br>**7.03 МБ** (NSIS Setup) | Скомпилировано с `-ldflags="-s -w -H=windowsgui"` | ✅ **PASS** |
| **Покрытие ядра тестами** | `≥ 80.0% line` | **83.2% statements** | `go test -coverprofile=coverage.out ./internal/core/...` (все пакеты ядра $\ge 80.0\%$) | ✅ **PASS** |
| **Anti-AI-Slop чистота** | 0 нарушений | **0 нарушений** | Проверка `scripts/anti_slop_lint.js` по TypeScript и Go коду | ✅ **PASS** |

---

## 4. Инвентаризация пакетов ядра и результаты покрытия тестами

| Пакет | Слой архитектуры | Тестовый статус | Coverage % | Ключевой функционал |
|---|---|---|---|---|
| `internal/core/auth` | Hexagonal Core | ✅ PASS | 81.2% | Microsoft OAuth2 PKCE, XSTS, Mojang UUID v3, Keyring |
| `internal/core/clock` | Hexagonal Core | ✅ PASS | 100.0% | Real & Mock Clock провайдеры |
| `internal/core/content` | Hexagonal Core | ✅ PASS | 80.5% | Парсер и экстрактор `.mrpack` |
| `internal/core/content/curseforge` | Hexagonal Core | ✅ PASS | 80.1% | CurseForge Core API клиент |
| `internal/core/content/loaders` | Hexagonal Core | ✅ PASS | 81.4% | Метаданные Fabric, Quilt, NeoForge |
| `internal/core/content/modrinth` | Hexagonal Core | ✅ PASS | 85.0% | Modrinth v2 API клиент |
| `internal/core/content/resolver` | Hexagonal Core | ✅ PASS | 90.8% | Разрешение графа зависимостей модов |
| `internal/core/downloader` | Hexagonal Core | ✅ PASS | 80.1% | Range-загрузчик с возобновлением и пулом соединений |
| `internal/core/game` | Hexagonal Core | ✅ PASS | 85.5% | Провижининг игры, Mojang V2, client.jar, libraries, assets |
| `internal/core/java` | Hexagonal Core | ✅ PASS | 82.0% | Матрица эпох Java (8/17/21) и Adoptium API v3 |
| `internal/core/launch` | Hexagonal Core | ✅ PASS | 84.8% | JVM аргументы, Log4j crash classifier, супервизия |
| `internal/core/security` | Hexagonal Core | ✅ PASS | 86.9% | Аудит открытых токенов в SQLite, PII-санитизация логов |
| `internal/core/storage` | Hexagonal Core | ✅ PASS | 80.3% | Pure-Go SQLite WAL (`modernc.org/sqlite`) + Goose миграции |
| `internal/core/updater` | Hexagonal Core | ✅ PASS | 81.2% | Ed25519 криптографический верификатор обновлений |
| `internal/adapters/wails` | Adapters | ✅ PASS | 85.4% | Wails v3 IPC адаптер (145.3 нс/оп) |
| `internal/adapters/process` | Adapters | ✅ PASS | 82.1% | Win32 Job Objects (`KILL_ON_JOB_CLOSE`) |
| `internal/adapters/java` | Adapters | ✅ PASS | 84.6% | Детектор установленных JDK (Реестр Windows / POSIX) |
| `internal/adapters/keyring` | Adapters | ✅ PASS | 77.8% | Windows Credential Manager / Secret Service Keyring |

**Итог по ядру**: суммарное покрытие `internal/core/...` составляет **83.2% statements**, что с запасом превышает норматив $\ge 80.0\%$.

---

## 5. Артефакты релиза v0.1.0

Релиз v0.1.0 официально опубликован:
- **Тег**: [v0.1.0](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/tag/v0.1.0)
- **Контрольные суммы SHA-256**:
  - `NordLauncher.exe`: `20496cbf35a384c43ee2d2ac87e49d78ae39a31179b7d68bf62bbfce76741dfa`
  - `NordLauncher-Setup.exe`: `9dd7b7a31f0a5e79a64697f2ad04edd9ddde2911533f264aebe3b4efbb6c44ca`
  - `nord-launcher-v0.1.0-linux-amd64.tar.gz`: `74cc0f61225a9c22686dd9a3633380f6c7312a7bf15e335a4ecaf38635893f56`
  - `manifest-stable.json`: криптографически подписан Ed25519 ключом проекта.

Релиз v0.1.1 включает полный конвейер провижининга игры (`internal/core/game`), реальный запуск игрового процесса с возвратом валидного системного PID, интерактивную авторизацию Microsoft OAuth и обновлённый CI-гейт.
