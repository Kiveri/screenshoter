package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

type Config struct {
	PageLoadWait  int `json:"pageLoadWait"`  // Таймаут загрузки страницы (секунды)
	DriverTimeout int `json:"driverTimeout"` // Таймаут для ChromeDriver (секунды)
	MaxRetries    int `json:"maxRetries"`    // Максимальное количество попыток
	MaxWorkers    int `json:"maxWorkers"`    // Максимальное количество параллельных обработчиков
}

const (
	configFile      = "config.json"
	linksFile       = "links.txt"
	screenshotsDir  = "screenshots"
	failedLinksFile = "failed_links.txt"
	chromeDriver    = "chromedriver.exe" // Ищем в корне проекта
)

func main() {
	fmt.Println("🚀 Скрипт для создания скриншотов веб-страниц")
	fmt.Println("------------------------------------------")

	// Проверяем текущую директорию
	if wd, err := os.Getwd(); err == nil {
		fmt.Printf("📂 Текущая рабочая директория: %s\n", wd)
	}

	// Загрузка конфигурации
	fmt.Println("⚙️ Загружаем конфигурацию...")
	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}
	printConfig(cfg)

	// Проверка наличия chromedriver.exe в корне проекта
	fmt.Println("🔍 Проверяем наличие chromedriver.exe...")
	driverPath := filepath.Join(".", chromeDriver)
	absDriverPath, err := filepath.Abs(driverPath)
	if err != nil {
		log.Fatalf("❌ Ошибка получения абсолютного пути: %v", err)
	}

	if _, err := os.Stat(absDriverPath); os.IsNotExist(err) {
		log.Fatalf("❌ chromedriver.exe не найден по пути: %s\n"+
			"Убедитесь, что:\n"+
			"1. chromedriver.exe находится в корне проекта\n"+
			"2. Имя файла точно 'chromedriver.exe'\n"+
			"3. Файл не заблокирован антивирусом", absDriverPath)
	}
	fmt.Printf("✅ chromedriver.exe найден по пути: %s\n", absDriverPath)

	// Подготовка папок
	fmt.Println("📂 Подготавливаем рабочее пространство...")
	if err := prepareWorkspace(); err != nil {
		log.Fatalf("❌ Ошибка подготовки: %v", err)
	}

	// Чтение списка ссылок
	fmt.Println("🔗 Читаем список ссылок...")
	links, err := readLinksFromFile(linksFile)
	if err != nil {
		log.Fatalf("❌ Ошибка чтения файла с ссылками: %v", err)
	}

	if len(links) == 0 {
		log.Fatal("❌ Файл с ссылками пуст или не содержит валидных URL")
	}

	fmt.Printf("✅ Найдено %d ссылок для обработки\n\n", len(links))

	// Создаем каналы для работы
	jobs := make(chan string, len(links))
	results := make(chan processingResult, len(links))

	// Запускаем worker'ов
	var wg sync.WaitGroup
	for i := 0; i < cfg.MaxWorkers; i++ {
		wg.Add(1)
		go worker(i+1, jobs, results, &wg, cfg, driverPath)
	}

	// Отправляем задачи в канал
	for _, url := range links {
		jobs <- url
	}
	close(jobs)

	// Запускаем сборщик результатов
	go func() {
		wg.Wait()
		close(results)
	}()

	// Обрабатываем результаты
	var (
		successCount int
		failedLinks  []string
	)

	fmt.Println("\n🛠️ Начинаем обработку ссылок...")
	for result := range results {
		if result.Error != nil {
			log.Printf("❌ Ошибка обработки [%s]: %v", result.URL, result.Error)
			failedLinks = append(failedLinks, result.URL)
		} else {
			log.Printf("✅ Успешно обработано [%s]", result.URL)
			successCount++
		}
	}

	// Сохраняем необработанные ссылки
	if len(failedLinks) > 0 {
		fmt.Printf("\n⚠️ Сохраняем %d необработанных ссылок...\n", len(failedLinks))
		if err := saveFailedLinks(failedLinks); err != nil {
			log.Printf("⚠️ Не удалось сохранить список необработанных ссылок: %v", err)
		}
	}

	fmt.Printf("\n🎉 Обработка завершена!\nУспешно: %d | Не удалось: %d\n", successCount, len(failedLinks))
	if len(failedLinks) > 0 {
		fmt.Printf("Необработанные ссылки сохранены в: %s\n", failedLinksFile)
	}
}

type processingResult struct {
	URL   string
	Error error
}

func worker(id int, jobs <-chan string, results chan<- processingResult, wg *sync.WaitGroup, cfg *Config, driverPath string) {
	defer wg.Done()

	for url := range jobs {
		log.Printf("👷 Worker #%d начал обработку: %s", id, url)
		var err error

		for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
			if attempt > 1 {
				log.Printf("🔄 Worker #%d попытка %d/%d для: %s", id, attempt, cfg.MaxRetries, url)
			}

			// Создаем новый экземпляр драйвера для каждой попытки
			port, cmd, driverErr := startChromeDriver(driverPath, time.Duration(cfg.DriverTimeout)*time.Second)
			if driverErr != nil {
				err = fmt.Errorf("не удалось запустить ChromeDriver: %v", driverErr)
				continue
			}

			// Обрабатываем URL
			processErr := processURL(url, port, cfg.PageLoadWait)

			// Закрываем драйвер
			if cmd.Process != nil {
				cmd.Process.Kill()
			}

			if processErr == nil {
				err = nil
				break
			}

			err = processErr
		}

		results <- processingResult{URL: url, Error: err}
	}
}

func processURL(url string, port int, pageLoadWait int) error {
	// Настройка Chrome
	chromeCaps := chrome.Capabilities{
		Args: []string{
			"--headless",
			"--disable-gpu",
			"--window-size=1280,1024",
		},
	}

	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chromeCaps)

	// Подключаемся к ChromeDriver
	wd, err := selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		return fmt.Errorf("ошибка подключения к ChromeDriver: %v", err)
	}
	defer wd.Quit()

	// Открываем страницу
	if err := wd.Get(url); err != nil {
		return fmt.Errorf("ошибка загрузки страницы: %v", err)
	}

	time.Sleep(time.Duration(pageLoadWait) * time.Second)

	// Создаем скриншот
	filename := generateFilename(url)
	screenshotPath := filepath.Join(screenshotsDir, filename)

	screenshot, err := wd.Screenshot()
	if err != nil {
		return fmt.Errorf("ошибка создания скриншота: %v", err)
	}

	if err := os.WriteFile(screenshotPath, screenshot, 0644); err != nil {
		return fmt.Errorf("ошибка сохранения скриншота: %v", err)
	}

	return nil
}

func generateFilename(url string) string {
	now := time.Now().UTC().Add(3 * time.Hour)
	timestamp := now.Format("2006-01-02_15-04-05.000") // ЧЧ:ММ_ДД.ММ.ГГГГ

	cleanURL := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"/", "_",
		"?", "_",
		"=", "_",
		"&", "_",
		":", "_",
		" ", "_",
		".", "_",
		",", "_",
	).Replace(url)

	if len(cleanURL) > 100 {
		cleanURL = cleanURL[:100]
	}

	randNum := rand.Intn(1000)

	return fmt.Sprintf("screenshot_%s_%s_%d.png", timestamp, cleanURL, randNum)
}

func startChromeDriver(driverPath string, timeout time.Duration) (int, *exec.Cmd, error) {
	// Получаем абсолютный путь к chromedriver.exe
	absDriverPath, err := filepath.Abs(driverPath)
	if err != nil {
		return 0, nil, fmt.Errorf("ошибка получения абсолютного пути: %v", err)
	}

	// Проверяем существование файла
	if _, err := os.Stat(absDriverPath); os.IsNotExist(err) {
		return 0, nil, fmt.Errorf("chromedriver.exe не найден по пути: %s", absDriverPath)
	}

	port, err := getFreePort()
	if err != nil {
		return 0, nil, fmt.Errorf("не удалось найти свободный порт: %v", err)
	}

	// Явно указываем полный путь к исполняемому файлу
	cmd := exec.Command(absDriverPath, fmt.Sprintf("--port=%d", port))
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("не удалось запустить ChromeDriver: %v (полный путь: %s)", err, absDriverPath)
	}

	if err := waitForChromeDriver(port, timeout); err != nil {
		cmd.Process.Kill()
		return 0, nil, err
	}

	return port, cmd, nil
}

func waitForChromeDriver(port int, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 1*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ChromeDriver не запустился за %v", timeout)
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Установка значений по умолчанию
	if cfg.PageLoadWait <= 0 {
		cfg.PageLoadWait = 5
	}
	if cfg.DriverTimeout <= 0 {
		cfg.DriverTimeout = 10
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 5
	}

	return &cfg, nil
}

func prepareWorkspace() error {
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать папку для скриншотов: %v", err)
	}

	// Очищаем файл с необработанными ссылками
	if err := os.WriteFile(failedLinksFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("не удалось инициализировать файл для необработанных ссылок: %v", err)
	}

	return nil
}

func readLinksFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var links []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && (strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")) {
			links = append(links, line)
		}
	}

	return links, scanner.Err()
}

func saveFailedLinks(links []string) error {
	content := strings.Join(links, "\n")
	return os.WriteFile(failedLinksFile, []byte(content), 0644)
}

func printConfig(cfg *Config) {
	fmt.Printf(`
Конфигурация:
- Таймаут загрузки страницы: %d сек
- Таймаут ChromeDriver: %d сек
- Макс. попыток: %d
- Макс. параллельных задач: %d
`,
		cfg.PageLoadWait,
		cfg.DriverTimeout,
		cfg.MaxRetries,
		cfg.MaxWorkers)
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}
