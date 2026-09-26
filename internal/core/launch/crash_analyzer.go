package launch

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

type CrashCategory string

const (
	CrashCategoryNone          CrashCategory = "none"
	CrashCategoryOOM           CrashCategory = "out_of_memory"
	CrashCategoryGraphics      CrashCategory = "graphics_driver_opengl"
	CrashCategoryModConflict   CrashCategory = "mod_conflict"
	CrashCategoryJavaVersion   CrashCategory = "java_version_mismatch"
	CrashCategoryCorruptedFile CrashCategory = "corrupted_archive"
	CrashCategoryUnknown       CrashCategory = "unknown"
)

type CrashReport struct {
	Category      CrashCategory `json:"category"`
	Summary       string        `json:"summary"`
	Remedy        string        `json:"remedy"`
	Details       string        `json:"details"`
	RelevantLines []string      `json:"relevant_lines"`
	ExitCode      int           `json:"exit_code"`
}

const DefaultLogBufferSize = 5000

// LogSupervisor streams and monitors Minecraft output, retaining ring-buffer of recent lines
// and automatically detecting fatal errors.
type LogSupervisor struct {
	mu           sync.RWMutex
	recentLines  []string
	maxLines     int
	detectedCat  CrashCategory
	detectedMsg  string
	offendingMod string
	listenerMu   sync.RWMutex
	listeners    []chan string
}

func NewLogSupervisor(bufferSize int) *LogSupervisor {
	if bufferSize <= 0 {
		bufferSize = DefaultLogBufferSize
	}
	return &LogSupervisor{
		recentLines: make([]string, 0, bufferSize),
		maxLines:    bufferSize,
		detectedCat: CrashCategoryNone,
		listeners:   make([]chan string, 0),
	}
}

// Subscribe registers a listener channel to receive real-time log lines.
// It returns the receive-only channel and an unsubscribe cleanup function.
// Channels use non-blocking send with drop policy so slow consumers never block execution.
func (s *LogSupervisor) Subscribe(bufLen int) (<-chan string, func()) {
	if bufLen <= 0 {
		bufLen = 256
	}
	ch := make(chan string, bufLen)

	s.listenerMu.Lock()
	s.listeners = append(s.listeners, ch)
	s.listenerMu.Unlock()

	unsubscribe := func() {
		s.listenerMu.Lock()
		defer s.listenerMu.Unlock()
		for i, l := range s.listeners {
			if l == ch {
				s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
				close(ch)
				break
			}
		}
	}
	return ch, unsubscribe
}

var (
	oomRegex       = regexp.MustCompile(`(?i)java\.lang\.OutOfMemoryError|insufficient memory for the Java Runtime`)
	openglRegex    = regexp.MustCompile(`(?i)Pixel format not accelerated|GLFW error 65542|WGL: The driver does not appear to support OpenGL|OpenGL 3\.2`)
	modRegex       = regexp.MustCompile(`(?i)net\.fabricmc\.loader\.impl\.FormattedException|Missing or incompatible mod|DuplicateModsFoundException|Mixin apply failed|ModLoadingException`)
	javaVerRegex   = regexp.MustCompile(`(?i)UnsupportedClassVersionError: .* compiled by a more recent version`)
	corruptRegex   = regexp.MustCompile(`(?i)ZipException: invalid LOC header|corrupt jar|MALFORMED`)
	fabricModRegex = regexp.MustCompile(`net\.fabricmc\.loader\.impl\.FormattedException: Mod '([^']+)'`)
)

func (s *LogSupervisor) ProcessLine(line string) {
	s.mu.Lock()

	if len(s.recentLines) >= s.maxLines {
		s.recentLines = s.recentLines[1:]
	}
	s.recentLines = append(s.recentLines, line)

	if s.detectedCat == CrashCategoryNone {
		if oomRegex.MatchString(line) {
			s.detectedCat = CrashCategoryOOM
			s.detectedMsg = line
		} else if openglRegex.MatchString(line) {
			s.detectedCat = CrashCategoryGraphics
			s.detectedMsg = line
		} else if javaVerRegex.MatchString(line) {
			s.detectedCat = CrashCategoryJavaVersion
			s.detectedMsg = line
		} else if corruptRegex.MatchString(line) {
			s.detectedCat = CrashCategoryCorruptedFile
			s.detectedMsg = line
		} else if modRegex.MatchString(line) {
			s.detectedCat = CrashCategoryModConflict
			s.detectedMsg = line
			if match := fabricModRegex.FindStringSubmatch(line); len(match) > 1 {
				s.offendingMod = match[1]
			}
		}
	}
	s.mu.Unlock()

	// Non-blocking send with drop policy outside mu
	s.listenerMu.RLock()
	var targets []chan string
	if len(s.listeners) > 0 {
		targets = make([]chan string, len(s.listeners))
		copy(targets, s.listeners)
	}
	s.listenerMu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- line:
		default:
			// slow consumer drop policy
		}
	}
}

// GetTail returns up to the last n lines from the supervisor's ring buffer.
// If n <= 0 or n > total, it returns all currently buffered lines.
// It returns a safe copy of the lines, or an empty slice if the buffer is empty.
func (s *LogSupervisor) GetTail(n int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.recentLines)
	if total == 0 {
		return []string{}
	}

	if n <= 0 || n > total {
		n = total
	}

	start := total - n
	res := make([]string, n)
	copy(res, s.recentLines[start:])
	return res
}

// AttachPipes starts scanning stdout and stderr in background goroutines.
func (s *LogSupervisor) AttachPipes(stdout, stderr io.Reader) {
	if stdout != nil {
		go s.scanReader(stdout)
	}
	if stderr != nil {
		go s.scanReader(stderr)
	}
}

func (s *LogSupervisor) scanReader(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.ProcessLine(scanner.Text())
	}
}

func (s *LogSupervisor) AnalyzeCrash(exitCode int) *CrashReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if exitCode == 0 && s.detectedCat == CrashCategoryNone {
		return nil // Clean exit
	}

	report := &CrashReport{
		ExitCode:      exitCode,
		RelevantLines: append([]string(nil), s.recentLines...),
	}

	cat := s.detectedCat
	if cat == CrashCategoryNone {
		// Try scan recent lines again in case error pattern was split
		fullText := strings.Join(s.recentLines, "\n")
		if oomRegex.MatchString(fullText) {
			cat = CrashCategoryOOM
		} else if openglRegex.MatchString(fullText) {
			cat = CrashCategoryGraphics
		} else if modRegex.MatchString(fullText) {
			cat = CrashCategoryModConflict
		} else if javaVerRegex.MatchString(fullText) {
			cat = CrashCategoryJavaVersion
		} else if corruptRegex.MatchString(fullText) {
			cat = CrashCategoryCorruptedFile
		} else {
			cat = CrashCategoryUnknown
		}
	}

	report.Category = cat

	switch cat {
	case CrashCategoryOOM:
		report.Summary = "Игра завершилась из-за нехватки оперативной памяти (Out of Memory)"
		report.Remedy = "Увеличьте объем выделенной оперативной памяти (RAM) в настройках инстанса (например, с 2048 МБ до 4096 МБ или 6144 МБ)."
	case CrashCategoryGraphics:
		report.Summary = "Ошибка графической подсистемы OpenGL / видеодрайвера"
		report.Remedy = "Обновите драйверы видеокарты (NVIDIA, AMD или Intel) с официального сайта производителя. Проверьте, что игра запускается на дискретной видеокарте, а не на базовом видеоадаптере Microsoft."
	case CrashCategoryModConflict:
		if s.offendingMod != "" {
			report.Summary = fmt.Sprintf("Конфликт модификаций с участием мода '%s'", s.offendingMod)
		} else {
			report.Summary = "Конфликт или несовместимость установленных модов"
		}
		report.Remedy = "Проверьте совместимость модов с вашей версией игры/загрузчика (Fabric/NeoForge) и убедитесь в наличии всех требуемых библиотек (например, Fabric API)."
	case CrashCategoryJavaVersion:
		report.Summary = "Несовместимая версия Java Runtime"
		report.Remedy = "Для запуска этой версии Minecraft требуется более новая версия Java (Java 17 для Minecraft 1.18–1.20.4, Java 21 для Minecraft 1.20.5+)."
	case CrashCategoryCorruptedFile:
		report.Summary = "Поврежден JAR-архив библиотеки или мода"
		report.Remedy = "Удалите поврежденный файл или выполните повторную синхронизацию ассетов в лаунчере."
	default:
		report.Summary = fmt.Sprintf("Игра аварийно завершилась с кодом ошибки %d", exitCode)
		report.Remedy = "Ознакомьтесь с журналом логов выше для выяснения подробностей сбоя."
	}

	report.Details = s.detectedMsg
	return report
}