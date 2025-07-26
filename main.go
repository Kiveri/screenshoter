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
}

const (
	configFile     = "config.json"
	linksFile      = "links.txt"
	screenshotsDir = "screenshots"
	chromeDriver   = "chromedriver.exe"
)

func main() {
	fmt.Println("Скрипт для создания скриншотов веб-страниц")
	fmt.Println("------------------------------------------")

	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	if err = os.MkdirAll(screenshotsDir, 0755); err != nil {
		log.Fatalf("Ошибка создания папки для скриншотов: %v", err)
	}

	// Получаем путь к текущей директории
	exeDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Ошибка получения текущей директории: %v", err)
	}

	fmt.Println("Поиск chromedriver.exe...")
	driverPath := filepath.Join(exeDir, chromeDriver)
	if _, err = os.Stat(driverPath); os.IsNotExist(err) {
		log.Fatalf("Файл chromedriver.exe не найден в директории: %s", exeDir)
	}

	fmt.Println("Запуск ChromeDriver...")
	port, cmd, err := startChromeDriver(driverPath, time.Duration(cfg.DriverTimeout)*time.Second)
	if err != nil {
		log.Fatalf("Ошибка запуска ChromeDriver: %v", err)
	}
	defer func() {
		fmt.Println("Завершение работы ChromeDriver...")
		cmd.Process.Kill()
	}()

	fmt.Printf("ChromeDriver успешно запущен на порту %d\n", port)

	chromeCaps := chrome.Capabilities{
		Args: []string{
			"--headless",
			"--disable-gpu",
			"--window-size=1280,1024",
		},
	}

	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chromeCaps)

	fmt.Println("Подключение к ChromeDriver...")
	wd, err := selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		log.Fatalf("Не удалось подключиться к ChromeDriver: %v", err)
	}
	defer wd.Quit()
	fmt.Println("Успешное подключение к ChromeDriver!")

	links, err := readLinksFromFile(linksFile)
	if err != nil {
		log.Fatalf("Ошибка чтения файла с ссылками: %v", err)
	}

	if len(links) == 0 {
		log.Fatal("Файл с ссылками пуст или не содержит валидных URL")
	}

	fmt.Printf("Найдено %d ссылок для обработки\n", len(links))

	for i, url := range links {
		fmt.Printf("\n[%d/%d] Обработка: %s\n", i+1, len(links), url)

		fmt.Println("Открытие страницы...")
		if err = wd.Get(url); err != nil {
			log.Printf("Ошибка при открытии страницы: %v\n", err)
			continue
		}

		waitTime := time.Duration(cfg.PageLoadWait) * time.Second
		fmt.Printf("Ожидание загрузки (%v)...\n", waitTime)
		time.Sleep(waitTime)

		filename := generateFilename(url)
		screenshotPath := filepath.Join(screenshotsDir, filename)

		fmt.Println("Создание скриншота...")

		var screenshot []byte
		screenshot, err = wd.Screenshot()
		if err != nil {
			log.Printf("Ошибка при создании скриншота: %v\n", err)
			continue
		}

		if err = os.WriteFile(screenshotPath, screenshot, 0644); err != nil {
			log.Printf("Ошибка при сохранении скриншота: %v\n", err)
			continue
		}

		fmt.Printf("Скриншот успешно сохранен: %s\n", screenshotPath)
	}

	fmt.Println("\nОбработка завершена!")
}

func startChromeDriver(driverPath string, timeout time.Duration) (int, *exec.Cmd, error) {
	port, err := getFreePort()
	if err != nil {
		return 0, nil, fmt.Errorf("не удалось найти свободный порт: %v", err)
	}

	cmd := exec.Command(driverPath, fmt.Sprintf("--port=%d", port))
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("не удалось запустить ChromeDriver: %v", err)
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
		cfg.DriverTimeout = 10
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
