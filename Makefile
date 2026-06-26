APP_NAME := predictor
SVC_NAME := $(APP_NAME).service
CFG_NAME := $(APP_NAME).json 
PRJ_NAME := collector
BUILD_DIR := ./bin
CFG_SRC_DIR := ./configs
SVC_SRC_DIR := ./service
CONFIG_DIR := /etc/$(PRJ_NAME)/configs
INSTALL_DIR := /etc/$(PRJ_NAME)
SYSTEMD_DIR := /etc/systemd/system

GREEN := \033[0;32m
RED := \033[0;31m
YELLOW := \033[0;33m
NC := \033[0m

.PHONY: all clean build

all: build

build:
	@echo "$(GREEN)Building $(APP_NAME)...$(NC)"
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME) main.go
	@echo "$(GREEN)Build complete: $(BUILD_DIR)/$(APP_NAME)$(NC)"

clean:
	@echo "$(YELLOW)Cleanning...$(NC)"
	rm -rf $(BUILD_DIR)
	@echo "$(GREEN)Clean complete$(NC)"

stop-service:
	@if systemctl is-active --quiet $(SVC_NAME); then \
		echo "$(YELLOW)Stopping service $(SVC_NAME)...$(NC)"; \
		systemctl stop $(SVC_NAME); \
		echo "$(GREEN)Service stopped$(NC)"; \
	else \
		echo "$(YELLOW)Service $(SVC_NAME) is not running$(NC)"; \
	fi

install: stop-service
	@echo "$(YELLOW)Installing $(APP_NAME)...$(NC)"
	
	# Создание необходимых директорий
	mkdir -p $(INSTALL_DIR)
	mkdir -p $(CONFIG_DIR)
	
	# Копирование бинарного файла
	cp $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)
	chmod 755 $(INSTALL_DIR)/$(APP_NAME)
	@echo "$(GREEN)Binary installed to $(INSTALL_DIR)/$(APP_NAME)$(NC)"
	
	# Копирование конфига (если существует)
	@if [ -f $(CFG_SRC_DIR)/$(CFG_NAME) ]; then \
		cp $(CFG_SRC_DIR)/$(CFG_NAME) $(CONFIG_DIR)/; \
		chmod 644 $(CONFIG_DIR)/$(CFG_NAME); \
		echo "$(GREEN)Config installed to $(CONFIG_DIR)/$(CFG_NAME)$(NC)"; \
	else \
		echo "$(YELLOW)Warning: $(CFG_NAME) not found, skipping...$(NC)"; \
	fi
	
	# Копирование systemd сервиса (если существует)
	@if [ -f $(SVC_SRC_DIR)/$(SVC_NAME) ]; then \
		cp $(SVC_SRC_DIR)/$(SVC_NAME) $(SYSTEMD_DIR)/; \
		chmod 644 $(SYSTEMD_DIR)/$(SVC_NAME); \
		echo "$(GREEN)Service file installed to $(SYSTEMD_DIR)/$(SVC_NAME)$(NC)"; \
	else \
		echo "$(YELLOW)Warning: $(SVC_NAME) not found, skipping...$(NC)"; \
	fi
	
	# Перезагрузка systemd и запуск сервиса
	systemctl daemon-reload
	systemctl enable $(SVC_NAME)
	systemctl start $(SVC_NAME)
	
	@echo "$(GREEN)Installation complete!$(NC)"
	@echo "$(GREEN)Service $(SVC_NAME) is now running$(NC)"