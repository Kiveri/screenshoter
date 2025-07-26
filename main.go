package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

const (
	linksFile      = "links.txt"        // Файл с ссылками
	screenshotsDir = "screenshots"      // Папка для скриншотов
	chromeDriver   = "chromedriver.exe" // Путь к chromedriver
	chromePort     = 63326              // Порт для ChromeDriver
	pageLoadWait   = 5 * time.Second    // Время ожидания загрузки страницы
)

func main() {
	fmt.Println("Скрипт для создания скриншотов веб-страниц")
	fmt.Println("------------------------------------------")

	// Проверяем и создаем папку для скриншотов
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		log.Fatalf("Ошибка создания папки для скриншотов: %v", err)
	}

	// Читаем ссылки из файла
	links, err := readLinksFromFile(linksFile)
	if err != nil {
		log.Fatalf("Ошибка чтения файла с ссылками: %v", err)
	}

	if len(links) == 0 {
		log.Fatal("Файл с ссылками пуст или не содержит валидных URL")
	}

	fmt.Printf("Найдено %d ссылок для обработки\n", len(links))

	// Настройки Chrome
	chromeCaps := chrome.Capabilities{
		Path: "", // Путь к Chrome (если не в PATH)
		Args: []string{
			"--headless", // Режим без отображения браузера
			"--disable-gpu",
			"--window-size=1280,1024",
		},
	}

	// Конфигурация WebDriver
	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chromeCaps)

	// Запускаем ChromeDriver
	wd, err := selenium.NewRemote(caps, "http://localhost:63326")
	if err != nil {
		log.Fatalf("Не удалось подключиться к ChromeDriver: %v", err)
	}
	defer wd.Quit()

	// Обрабатываем каждую ссылку
	for i, url := range links {
		fmt.Printf("[%d/%d] Обработка: %s\n", i+1, len(links), url)

		// Открываем URL
		if err := wd.Get(url); err != nil {
			log.Printf("Ошибка при открытии страницы: %v\n", err)
			continue
		}

		// Ждем загрузки страницы
		time.Sleep(pageLoadWait)

		// Генерируем имя файла для скриншота
		filename := generateFilename(url)
		screenshotPath := filepath.Join(screenshotsDir, filename)

		// Делаем скриншот
		screenshot, err := wd.Screenshot()
		if err != nil {
			log.Printf("Ошибка при создании скриншота: %v\n", err)
			continue
		}

		// Сохраняем скриншот
		if err := os.WriteFile(screenshotPath, screenshot, 0644); err != nil {
			log.Printf("Ошибка при сохранении скриншота: %v\n", err)
			continue
		}

		fmt.Printf("Скриншот сохранен: %s\n", screenshotPath)
	}

	fmt.Println("Обработка завершена!")
}

// Чтение ссылок из файла
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

// Генерация имени файла для скриншота
func generateFilename(url string) string {
	// Удаляем протокол и специальные символы
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

	// Ограничиваем длину имени файла
	if len(cleanURL) > 50 {
		cleanURL = cleanURL[:50]
	}

	// Добавляем временную метку
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.png", cleanURL, timestamp)
}
