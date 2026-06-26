package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
	"predictor/utils"

	"github.com/getumen/go-treelite"
)

type Predictor struct {
	log         *utils.Logger
	config      *Config
	buffer      *CircularBuffer
	subscribers map[net.Conn]chan bool
	subsMu      sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	listener    net.Listener
	conn        net.Conn
	model       *treelite.Model
	predictor   *treelite.Predictor
}

func NewPredictor(cfg *Config) *Predictor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Predictor{
		log:         utils.GlobalLogger(),
		config:      cfg,
		buffer:      NewCircularBuffer(cfg.BufferSize),
		subscribers: make(map[net.Conn]chan bool),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (p *Predictor) extractFeatures(data DataPoint) ([]float32, error) {
	features := make([]float32, 0, len(p.config.DataSources.Parameters))
	
	for _, paramName := range p.config.DataSources.Parameters {
		val, exists := data.Values[paramName]
		if !exists {
			return nil, fmt.Errorf("parameter %s not found in data", paramName)
		}
		
		var floatVal float64
		switch v := val.(type) {
		case float64:
			floatVal = v
		case float32:
			floatVal = float64(v)
		case int:
			floatVal = float64(v)
		case int64:
			floatVal = float64(v)
		default:
			return nil, fmt.Errorf("parameter %s has unsupported type: %T", paramName, v)
		}
		
		features = append(features, float32(floatVal))
	}
	
	return features, nil
}

func (p *Predictor) getActualValue(data DataPoint) (float64, bool) {
	val, exists := data.Values[p.config.DataSources.Predict]
	if !exists {
		return 0, false
	}
	
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func (p *Predictor) connectToNormalizer() error {
	conn, err := net.Dial("unix", p.config.UDSNormalizerPath)
	if err != nil {
		return fmt.Errorf("failed to connect to normalizer: %v", err)
	}
	p.conn = conn
	return nil
}

func (p *Predictor) subscribeToNormalizer() error {
	_, err := p.conn.Write([]byte("SUBSCRIBE\n"))
	if err != nil {
		return fmt.Errorf("failed to subscribe: %v", err)
	}
	p.log.Info("Subscribed to normalizer")
	return nil
}

func (p *Predictor) receiveData() {
	defer p.wg.Done()
	
	decoder := json.NewDecoder(p.conn)
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			var raw RawDataPoint
			if err := decoder.Decode(&raw); err != nil {
				p.log.Error("Error receiving data from normalizer: %v", err)
				return
			}

			data := DataPoint{
				Timestamp: raw.Timestamp,
				Values:    raw.Values,
			}

			features, err := p.extractFeatures(data)
			if err != nil {
				p.log.Error("Error extracting features: %v", err)
				continue
			}

			// Создаем DMatrix из признаков
			// 1 строка (1 образец), n колонок (признаков)
			nrow := 1
			ncol := len(features)
			dmat, err := treelite.CreateFromMat(features, nrow, ncol, 0.0)
			if err != nil {
				p.log.Error("Error creating DMatrix: %v", err)
				continue
			}
			defer dmat.Close()

			// Делаем предсказание
			predictions, err := p.predictor.PredictBatch(dmat, false, false)
			if err != nil {
				p.log.Error("Error making prediction: %v", err)
				continue
			}

			if len(predictions) == 0 {
				p.log.Error("No predictions returned")
				continue
			}

			prediction := float64(predictions[0])
			actual, hasActual := p.getActualValue(data)

			predResult := PredictionResult{
				Timestamp:  data.Timestamp,
				Prediction: prediction,
			}
			if hasActual {
				predResult.Actual = actual
			}

			p.buffer.Push(predResult)
			p.notifySubscribers(predResult)
		}
	}
}

func (p *Predictor) handleConnection(conn net.Conn) {
	defer p.wg.Done()
	defer conn.Close()

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			p.removeSubscriber(conn)
			return
		}

		command := string(buf[:n])
		switch command {
		case "GET\n", "GET\r\n":
			allData := p.buffer.GetAll()
			response, _ := json.Marshal(allData)
			conn.Write(response)

		case "SUBSCRIBE\n", "SUBSCRIBE\r\n":
			p.log.Info("New subscriber to predictor")
			p.addSubscriber(conn)
			<-p.waitForUnsubscribe(conn)
			p.removeSubscriber(conn)
			return

		default:
			conn.Write([]byte("Unknown command\n"))
		}
	}
}

func (p *Predictor) notifySubscribers(result PredictionResult) {
	p.subsMu.RLock()
	defer p.subsMu.RUnlock()

	if len(p.subscribers) > 0 {
		data, err := json.Marshal(result)
		if err != nil {
			p.log.Error("Error marshaling prediction: %v", err)
			return
		}
		data = append(data, '\n')

		for conn := range p.subscribers {
			_, err := conn.Write(data)
			if err != nil {
				p.log.Error("Error sending prediction to subscriber: %v", err)
			}
		}
	}
}

func (p *Predictor) addSubscriber(conn net.Conn) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	p.subscribers[conn] = make(chan bool)
}

func (p *Predictor) removeSubscriber(conn net.Conn) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	delete(p.subscribers, conn)
}

func (p *Predictor) waitForUnsubscribe(conn net.Conn) chan bool {
	ch := make(chan bool)
	go func() {
		buf := make([]byte, 1)
		conn.Read(buf)
		close(ch)
	}()
	return ch
}

func (p *Predictor) Start() error {
	// Загружаем модель XGBoost
	model, err := treelite.LoadXGBoostModel(p.config.DataSources.ModelPath)
	if err != nil {
		return fmt.Errorf("failed to load model: %v", err)
	}
	p.model = model
	p.log.Info("XGBoost model loaded successfully")

	// Создаем predictor из .so файла
	// Загружаем скомпилированную библиотеку
	predictor, err := treelite.NewPredictor(p.config.DataSources.ModelPath, 1)
	if err != nil {
		return fmt.Errorf("failed to create predictor: %v", err)
	}
	p.predictor = predictor
	p.log.Info("Predictor created successfully")
	p.log.Info("Model info - Features: %d, Classes: %d", predictor.NumFeature(), predictor.NumClass())

	if err := p.connectToNormalizer(); err != nil {
		return err
	}

	if err := p.subscribeToNormalizer(); err != nil {
		return err
	}

	os.Remove(p.config.UDSSocketPath)
	listener, err := net.Listen("unix", p.config.UDSSocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on UDS: %v", err)
	}
	p.listener = listener

	p.wg.Add(1)
	go p.receiveData()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.ctx.Done():
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					select {
					case <-p.ctx.Done():
						return
					default:
						continue
					}
				}
				p.wg.Add(1)
				go p.handleConnection(conn)
			}
		}
	}()

	p.log.Info("Predictor started successfully")
	return nil
}

func (p *Predictor) Stop() error {
	p.cancel()

	if p.predictor != nil {
		p.predictor.Close()
	}

	if p.model != nil {
		p.model.Close()
	}

	if p.conn != nil {
		p.conn.Close()
	}

	if p.listener != nil {
		p.listener.Close()
	}

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for goroutines to stop")
	}
}