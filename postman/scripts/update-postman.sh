#!/bin/bash
# Скрипт для обновления Postman коллекции
# Автоматически определяет ОС и пути

# Определяем имя коллекции
COLLECTION_NAME="News_API.postman_collection.json"

# Определяем корень проекта
if [ -d "/workspaces/APIv4" ]; then
    # GitHub Codespaces
    PROJECT_ROOT="/workspaces/APIv4"
elif [ -d "/c/Users" ]; then
    # Git Bash на Windows
    PROJECT_ROOT="/c/Users/$USERNAME/Desktop/APIv4"
else
    # Linux/macOS или другой путь
    PROJECT_ROOT=$(pwd)
    while [ ! -f "$PROJECT_ROOT/go.mod" ] && [ "$PROJECT_ROOT" != "/" ]; do
        PROJECT_ROOT=$(dirname "$PROJECT_ROOT")
    done
fi

POSTMAN_DIR="$PROJECT_ROOT/postman"
COLLECTION_PATH="$POSTMAN_DIR/$COLLECTION_NAME"

# Определяем путь для загрузок в зависимости от ОС
case "$(uname -s)" in
    Linux*)
        if [ -d "/workspaces" ]; then
            # Codespaces
            DOWNLOAD_PATH="$HOME/Downloads"
        else
            # Обычный Linux
            DOWNLOAD_PATH="$HOME/Downloads"
        fi
        ;;
    Darwin*)
        # macOS
        DOWNLOAD_PATH="$HOME/Downloads"
        ;;
    CYGWIN*|MINGW*|MSYS*)
        # Windows Git Bash
        DOWNLOAD_PATH="/c/Users/$USERNAME/Downloads"
        ;;
    *)
        DOWNLOAD_PATH="$HOME"
        ;;
esac

echo "🌐 Система: $(uname -s)"
echo "📁 Проект: $PROJECT_ROOT"
echo "📥 Загрузки: $DOWNLOAD_PATH"
echo "🔄 Обновление Postman коллекции..."
echo "================================="

# Проверка зависимостей
if ! command -v python3 &> /dev/null; then
    echo "⚠️  Python3 не установлен, проверка JSON будет пропущена"
    CHECK_JSON=false
else
    CHECK_JSON=true
fi

# Функция для проверки JSON
check_json() {
    local file="$1"
    if $CHECK_JSON; then
        python3 -m json.tool "$file" > /dev/null 2>&1
        return $?
    else
        # Простая проверка что это JSON
        grep -q '^{' "$file" && grep -q '}$' "$file"
        return $?
    fi
}

# Основная логика
if [ $# -eq 1 ] && [ -f "$1" ]; then
    # Использовать файл из аргумента
    SOURCE_FILE="$1"
    echo "📂 Использую указанный файл: $SOURCE_FILE"
elif [ -f "$DOWNLOAD_PATH/$COLLECTION_NAME" ]; then
    # Использовать файл из Downloads
    SOURCE_FILE="$DOWNLOAD_PATH/$COLLECTION_NAME"
    echo "📦 Найден файл в Downloads"
else
    echo "❌ Файл коллекции не найден"
    echo ""
    echo "Поместите файл $COLLECTION_NAME в одну из папок:"
    echo "1. $DOWNLOAD_PATH"
    echo "2. $POSTMAN_DIR"
    echo ""
    echo "Или укажите путь при запуске:"
    echo "  ./update-postman.sh /путь/к/файлу.json"
    exit 1
fi

# Создать папку postman если нет
mkdir -p "$POSTMAN_DIR"

# Создать backup
if [ -f "$COLLECTION_PATH" ]; then
    TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
    BACKUP_FILE="$POSTMAN_DIR/${COLLECTION_NAME%.*}_backup_$TIMESTAMP.json"
    cp "$COLLECTION_PATH" "$BACKUP_FILE"
    echo "💾 Backup: $(basename "$BACKUP_FILE")"
fi

# Копировать файл
echo "📤 Копирую $SOURCE_FILE → $COLLECTION_PATH"
cp "$SOURCE_FILE" "$COLLECTION_PATH"

# Проверить JSON
if check_json "$COLLECTION_PATH"; then
    echo "✅ JSON валиден"
    echo "🎉 Коллекция обновлена успешно!"
    
    # Показать информацию о коллекции
    echo ""
    echo "📊 Информация о коллекции:"
    echo "--------------------------"
    if command -v jq &> /dev/null; then
        echo "Название: $(jq -r '.info.name' "$COLLECTION_PATH")"
        echo "Описание: $(jq -r '.info.description // "нет"' "$COLLECTION_PATH")"
        echo "Кол-во запросов: $(jq -r '.item | length' "$COLLECTION_PATH")"
    else
        echo "Размер: $(du -h "$COLLECTION_PATH" | cut -f1)"
        echo "Установите jq для подробной информации"
    fi
else
    echo "❌ Ошибка: файл не является валидным JSON"
    if [ -f "$BACKUP_FILE" ]; then
        echo "↩️  Восстанавливаю backup..."
        cp "$BACKUP_FILE" "$COLLECTION_PATH"
        echo "✅ Восстановлено"
    fi
    exit 1
fi

echo ""
echo "📁 Содержимое папки postman:"
ls -lh "$POSTMAN_DIR/"
EOF

chmod +x update-postman.sh
