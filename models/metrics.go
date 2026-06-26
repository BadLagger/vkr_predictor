package models

import (
	"sync"
)

type DataPoint struct {
	Timestamp int64                  `json:"timestamp"`
	Values    map[string]interface{} `json:"values"`
}

// RawDataPoint для получения данных от нормализатора
type RawDataPoint struct {
	Timestamp int64                  `json:"timestamp"`
	Values    map[string]interface{} `json:"values"`
}

// PredictionResult - результат прогнозирования
type PredictionResult struct {
	Timestamp  int64   `json:"timestamp"`
	Prediction float64 `json:"prediction"`
	Actual     float64 `json:"actual,omitempty"` // реальное значение если доступно
}

type CircularBuffer struct {
	data  []PredictionResult
	size  int
	head  int
	count int
	mu    sync.RWMutex
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data:  make([]PredictionResult, size),
		size:  size,
		head:  0,
		count: 0,
	}
}

func (cb *CircularBuffer) Push(point PredictionResult) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.data[cb.head] = point
	cb.head = (cb.head + 1) % cb.size
	if cb.count < cb.size {
		cb.count++
	}
}

func (cb *CircularBuffer) GetAll() []PredictionResult {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	result := make([]PredictionResult, cb.count)
	for i := 0; i < cb.count; i++ {
		idx := (cb.head - cb.count + i) % cb.size
		if idx < 0 {
			idx += cb.size
		}
		result[i] = cb.data[idx]
	}
	return result
}