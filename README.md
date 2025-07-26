# Screenshoter v1.0.6
Version: 1.0.6

![Go](https://img.shields.io/badge/Go-1.24+-blue)
![Platform](https://img.shields.io/badge/Platform-Windows-lightgrey)

Программа для автоматического создания скриншотов веб-страниц

## Как использовать

1. Скачайте последний релиз из [раздела Releases](https://github.com/Kiveri/screenshoter/releases)
2. Распакуйте архив `screenshoter.zip`
3. Отредактируйте файлы:
    - `links.txt` - список URL для скриншотов
    - `config.json` - настройки программы
4. Запустите `screenshoter.exe`
5. Не забудьте проверить файл failed_links.txt

## Состав релиза
- `screenshoter.exe` - основная программа
- `chromedriver.exe` - драйвер браузера
- `config.json` - конфигурация (таймауты и параметры)
- `links.txt` - пример файла с ссылками

## Требования
- Windows 10/11
- Google Chrome (последняя версия)

## Автоматическое обновление версии
Версия в этом файле автоматически обновляется при создании нового релиза через GitHub Actions.

<!-- Эти строки важны для автообновления -->
Current version: v1.0.4
<!-- End version marker -->
