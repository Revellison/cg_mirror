#!/bin/bash

# Цвета для вывода
GREEN='\033[00;32m'
RESTORE='\033[0m'

echo -e "${GREEN}>>> Начинаем установку зеркала ChainGate...${RESTORE}"

# 1. Проверка прав
if [ "$EUID" -ne 0 ]; then
  echo "Запустите скрипт от имени root (sudo)"
  exit
fi

# 2. Установка зависимостей
apt update && apt install -y curl git tar wget

# 3. Установка Go (если не установлен)
if ! command -v go &> /dev/null; then
    echo "Устанавливаем Go..."
    GO_VER="1.22.2" # Можно менять на актуальную
    wget "https://golang.org/dl/go${GO_VER}.linux-amd64.tar.gz"
    rm -rf /usr/local/go && tar -C /usr/local -xzf "go${GO_VER}.linux-amd64.tar.gz"
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    rm "go${GO_VER}.linux-amd64.tar.gz"
else
    echo "Go уже установлен: $(go version)"
fi

# 4. Подготовка файлов конфигурации
if [ ! -f config.yaml ]; then
    echo "Создаем config.yaml из примера..."
    cp config.example.yaml config.yaml
    echo "!!! Отредактируйте config.yaml перед запуском !!!"
fi

# 5. Сборка бинарника
echo "Собираем проект..."
/usr/local/go/bin/go mod tidy
/usr/local/go/bin/go build -ldflags="-s -w" -o mirror-app .

# 6. Настройка Systemd сервиса
CUR_DIR=$(pwd)
cat <<EOF > /etc/systemd/system/mirror.service
[Unit]
Description=ChainGate Mirror Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$CUR_DIR
ExecStart=$CUR_DIR/mirror-app
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# 7. Запуск
systemctl daemon-reload
systemctl enable mirror
systemctl start mirror

echo -e "${GREEN}>>> Готово! Сервис mirror запущен.${RESTORE}"
echo "Статус: systemctl status mirror"