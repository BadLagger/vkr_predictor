package main

import (
	"predictor/models"
	"os"
	"os/signal"
	"syscall"
	"predictor/utils"
)

func main() {
	log := utils.GlobalLogger() // используйте ваш логгер
	log.SetLevel(utils.Debug)
	log.Info("Start Predictor!")
	defer log.Info("Predictor Ends!")

	if len(os.Args) < 2 {
		log.Error("Should set path to the config file!")
		return
	}

	cfg, err := models.NewConfig(os.Args[1])
	if err != nil {
		log.Error("Config error: %v", err)
		return
	}

	predictor := models.NewPredictor(cfg)
	if err := predictor.Start(); err != nil {
		log.Error("Start predictor error: %v", err)
		os.Exit(1)
	}

	log.Info("Predictor started")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutting down gracefully...")

	if err := predictor.Stop(); err != nil {
		log.Error("Error during shutdown: %v", err)
		os.Exit(1)
	}
	log.Info("Shutdown completed")
}