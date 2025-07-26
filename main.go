package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

type Config struct {
	PageLoadWait  int `json:"pageLoadWait"`
	DriverTimeout int `json:"driverTimeout"`
	MaxRetries    int `json:"maxRetries"` // Максимальное количество попыток перезапуска
}

const (
	configFile      = "config.json"
	linksFile       = "links.txt"
	screenshotsDir  = "screenshots"
	failedLinksFile = "failed_links.txt" // Файл для необработанных ссылок
	chromeDriver    = "chromedriver.exe"
)

func main() {
	fmt.Println("Скрипт для создания скриншотов веб-страниц")
	fmt.Println("------------------------------------------")

	// Загрузка конфигурации
	fmt.Println("[1/7] Загрузка конфигурации...")
	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}
	fmt.Printf("Конфигурация загружена: ожидание загрузки - %dс, таймаут драйвера - %dс, попыток - %d\n",
		cfg.PageLoadWait, cfg.DriverTimeout, cfg.MaxRetries)

	// Создание папки для скриншотов
	fmt.Println("[2/7] Создание папки для скриншотов...")
	if err = os.MkdirAll(screenshotsDir, 0755); err != nil {
		log.Fatalf("Ошибка создания папки для скриншотов: %v", err)
	}

	// Инициализация файла для необработанных ссылок
	fmt.Println("[3/7] Инициализация файла для необработанных ссылок...")
	if err = os.WriteFile(failedLinksFile, []byte{}, 0644); err != nil {
		log.Printf("Ошибка создания файла для необработанных ссылок: %v", err)
	}

	// Поиск ChromeDriver
	fmt.Println("[4/7] Поиск chromedriver.exe...")
	exeDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Ошибка получения текущей директории: %v", err)
	}
	driverPath := filepath.Join(exeDir, chromeDriver)
	if _, err = os.Stat(driverPath); os.IsNotExist(err) {
		log.Fatalf("Файл chromedriver.exe не найден в директории: %s", exeDir)
	}

	// Чтение списка ссылок
	fmt.Println("[5/7] Чтение списка ссылок...")
	links, err := readLinksFromFile(linksFile)
	if err != nil {
		log.Fatalf("Ошибка чтения файла с ссылками: %v", err)
	}
	if len(links) == 0 {
		log.Fatal("Файл с ссылками пуст или не содержит валидных URL")
	}
	fmt.Printf("Найдено %d ссылок для обработки\n", len(links))

	// Основной цикл обработки
	fmt.Println("[6/7] Начало обработки ссылок...")
	for i, url := range links {
		fmt.Printf("\n[Ссылка %d/%d] Обработка: %s\n", i+1, len(links), url)

		retryCount := 0
		success := false

		for retryCount <= cfg.MaxRetries {
			fmt.Printf("\nПопытка %d/%d\n", retryCount+1, cfg.MaxRetries)

			// Запуск ChromeDriver
			fmt.Println(" - Запуск ChromeDriver...")

			var (
				port int
				cmd  *exec.Cmd
			)
			port, cmd, err = startChromeDriver(driverPath, time.Duration(cfg.DriverTimeout)*time.Second)
			if err != nil {
				log.Printf("Ошибка запуска ChromeDriver: %v", err)
				break
			}

			// Обработка URL
			fmt.Println(" - Обработка URL...")
			success = processUrl(url, port, cfg.PageLoadWait)
			cmd.Process.Kill() // Закрываем драйвер после каждой попытки

			if success {
				break
			}

			retryCount++
		}

		if !success {
			fmt.Println(" - Сохранение необработанной ссылки...")
			saveFailedLink(url)
		}
	}

	// Завершение работы
	fmt.Println("\n[7/7] Обработка завершена!")
	fmt.Println("------------------------------------------")
	fmt.Println("Могут быть необработанные ссылки, загляни в файл", failedLinksFile)
}

func processUrl(url string, port int, pageLoadWait int) bool {
	fmt.Println("   - Настройка Chrome...")
	chromeCaps := chrome.Capabilities{
		Args: []string{
			"--headless",
			"--disable-gpu",
			"--window-size=1280,1024",
		},
	}

	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chromeCaps)

	fmt.Println("   - Подключение к ChromeDriver...")
	wd, err := selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		log.Printf("Ошибка подключения к ChromeDriver: %v", err)
		return false
	}
	defer wd.Quit()

	fmt.Println("   - Открытие страницы...")
	if err = wd.Get(url); err != nil {
		log.Printf("Ошибка при открытии страницы: %v", err)
		return false
	}

	fmt.Printf("   - Ожидание загрузки (%dс)...\n", pageLoadWait)
	time.Sleep(time.Duration(pageLoadWait) * time.Second)

	fmt.Println("   - Генерация имени файла...")
	filename := generateFilename(url)
	screenshotPath := filepath.Join(screenshotsDir, filename)

	fmt.Println("   - Создание скриншота...")
	screenshot, err := wd.Screenshot()
	if err != nil {
		log.Printf("Ошибка при создании скриншота: %v", err)
		return false
	}

	fmt.Println("   - Сохранение скриншота...")
	if err = os.WriteFile(screenshotPath, screenshot, 0644); err != nil {
		log.Printf("Ошибка при сохранении скриншота: %v", err)
		return false
	}

	fmt.Printf("   - Скриншот сохранен: %s\n", screenshotPath)
	return true
}

func saveFailedLink(url string) {
	file, err := os.OpenFile(failedLinksFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Ошибка при сохранении необработанной ссылки: %v", err)
		return
	}
	defer file.Close()

	if _, err = file.WriteString(url + "\n"); err != nil {
		log.Printf("Ошибка записи в файл необработанных ссылок: %v", err)
	}
}

func startChromeDriver(driverPath string, timeout time.Duration) (int, *exec.Cmd, error) {
	port, err := getFreePort()
	if err != nil {
		return 0, nil, fmt.Errorf("не удалось найти свободный порт: %v", err)
	}

	cmd := exec.Command(driverPath, fmt.Sprintf("--port=%d", port))
	if err = cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("не удалось запустить ChromeDriver: %v", err)
	}

	if err = waitForChromeDriver(port, timeout); err != nil {
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
			fmt.Println("ChromeDriver готов к работе!")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ChromeDriver не запустился за %v", timeout)
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

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.PageLoadWait == 0 {
		cfg.PageLoadWait = 5
	}
	if cfg.DriverTimeout == 0 {
		cfg.DriverTimeout = 5
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5 // Значение по умолчанию
	}

	return &cfg, nil
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

func generateFilename(url string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	cleanURL := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"/", "_",
		"?", "_",
		"=", "_",
		"&", "_",
		":", "_",
		" ", "_",
	).Replace(url)

	if len(cleanURL) > 100 {
		cleanURL = cleanURL[:100]
	}

	return fmt.Sprintf("screenshot_%s_%s.png", timestamp, cleanURL)
}
