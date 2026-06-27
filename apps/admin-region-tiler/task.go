package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/teris-io/shortid"
	pb "gopkg.in/cheggaaa/pb.v1"
)

const MBTileVersion = "1.2"

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskPaused    TaskStatus = "paused"
	TaskCompleted TaskStatus = "completed"
	TaskCancelled TaskStatus = "cancelled"
	TaskFailed    TaskStatus = "failed"
)

type TaskOptions struct {
	WorkerCount    int
	SavePipeSize   int
	TimeDelay      int
	TimeJitter     int
	RetryPasses    int
	BufferSize     int
	OutputFormat   string
	MaxRetries     int
	RequestTTL     time.Duration
	RetryBackoff   time.Duration
	SlowBackoff    time.Duration
	MaxSlowBackoff time.Duration
	Policy         FetchPolicy
}

type Task struct {
	ID          string
	Name        string
	Description string
	File        string
	Min         int
	Max         int
	Layers      []Layer
	TileMap     TileMap

	Total        int64
	Current      int64
	SuccessCount int64
	FailureCount int64

	Status       TaskStatus
	ErrorMessage string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time

	Bar *pb.ProgressBar
	db  *sql.DB

	workerCount    int
	savePipeSize   int
	timeDelay      int
	timeJitter     int
	retryPasses    int
	bufSize        int
	outformat      string
	maxRetries     int
	requestTTL     time.Duration
	retryBackoff   time.Duration
	slowBackoff    time.Duration
	maxSlowBackoff time.Duration
	policy         FetchPolicy

	ctx    context.Context
	cancel context.CancelFunc

	jobs       chan TileJob
	savingpipe chan Tile
	tileWG     sync.WaitGroup
	saveWG     sync.WaitGroup

	mu             sync.RWMutex
	pauseCond      *sync.Cond
	throttleUntil  time.Time
	throttleLevel  int
	retryMu        sync.Mutex
	retryQueue     []TileJob
	failureMu      sync.Mutex
	failureRecords []TileFailureRecord
	clientMu       sync.Mutex
	clientCache    map[string]*http.Client
	proxyRotator   *proxyRotator
}

type TileJob struct {
	Tile maptile.Tile
	URL  string
	Pass int
}

type TileFailureRecord struct {
	Z            int
	X            int
	Y            int
	URL          string
	ErrorMessage string
	Retryable    bool
	Attempt      int
	CreatedAt    time.Time
}

type HTTPStatusError struct {
	StatusCode int
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status code: %d", e.StatusCode)
}

func NewTask(layers []Layer, m TileMap, opts TaskOptions) *Task {
	if len(layers) == 0 {
		return nil
	}

	id, _ := shortid.Generate()
	ctx, cancel := context.WithCancel(context.Background())

	task := &Task{
		ID:             id,
		Name:           m.Name,
		Description:    m.Description,
		Layers:         layers,
		Min:            m.Min,
		Max:            m.Max,
		TileMap:        m,
		Status:         TaskPending,
		CreatedAt:      time.Now(),
		workerCount:    maxInt(opts.WorkerCount, 1),
		savePipeSize:   maxInt(opts.SavePipeSize, 1),
		timeDelay:      maxInt(opts.TimeDelay, 0),
		timeJitter:     maxInt(opts.TimeJitter, 0),
		retryPasses:    maxInt(opts.RetryPasses, 0),
		bufSize:        maxInt(opts.BufferSize, 1),
		outformat:      opts.OutputFormat,
		maxRetries:     maxInt(opts.MaxRetries, 0),
		requestTTL:     opts.RequestTTL,
		retryBackoff:   opts.RetryBackoff,
		slowBackoff:    maxDuration(opts.SlowBackoff, 0),
		maxSlowBackoff: maxDuration(opts.MaxSlowBackoff, 0),
		policy:         opts.Policy,
		ctx:            ctx,
		cancel:         cancel,
		clientCache:    make(map[string]*http.Client),
		proxyRotator:   newProxyRotator(opts.Policy.Proxies),
	}

	if task.policy.BaseDelayMS > 0 {
		task.timeDelay = maxInt(task.policy.BaseDelayMS, 0)
	}
	if task.policy.TimeJitterMS > 0 {
		task.timeJitter = maxInt(task.policy.TimeJitterMS, 0)
	}
	if task.policy.WorkerCount > 0 {
		task.workerCount = maxInt(task.policy.WorkerCount, 1)
	}
	if task.policy.MaxRetriesSet {
		task.maxRetries = maxInt(task.policy.MaxRetries, 0)
	}
	if task.policy.RetryPassesSet {
		task.retryPasses = maxInt(task.policy.RetryPasses, 0)
	}

	task.pauseCond = sync.NewCond(&task.mu)
	task.jobs = make(chan TileJob, task.bufSize)
	task.savingpipe = make(chan Tile, task.savePipeSize)

	for i := 0; i < len(layers); i++ {
		if layers[i].URL == "" {
			layers[i].URL = m.URL
		}
		layers[i].Count = tilecover.CollectionCount(layers[i].Collection, maptile.Zoom(layers[i].Zoom))
		task.Total += layers[i].Count
	}

	return task
}

func buildTaskFromRequest(req CreateTaskRequest) (*Task, error) {
	layers := make([]Layer, 0)
	minZoom := 100
	maxZoom := -1
	for _, level := range req.Levels {
		collection, err := loadCollection(level.Geojson)
		if err != nil {
			return nil, err
		}

		levelURL := req.URL
		if strings.TrimSpace(level.URL) != "" {
			levelURL = strings.TrimSpace(level.URL)
		}

		for z := level.MinZoom; z <= level.MaxZoom; z++ {
			layers = append(layers, Layer{
				URL:        levelURL,
				Zoom:       z,
				Collection: collection,
			})
		}

		if level.MinZoom < minZoom {
			minZoom = level.MinZoom
		}
		if level.MaxZoom > maxZoom {
			maxZoom = level.MaxZoom
		}
	}

	tm := TileMap{
		Name:       req.Name,
		SourceName: req.SourceName,
		Min:        minZoom,
		Max:        maxZoom,
		Format:     req.Format,
		Schema:     req.Schema,
		URL:        req.URL,
	}

	policy := defaultFetchPolicy(req.URL, req.SourceName)
	tm.Policy = policy

	options := TaskOptions{
		WorkerCount:    firstPositive(req.Workers, viper.GetInt("task.workers")),
		SavePipeSize:   firstPositive(req.SavePipe, viper.GetInt("task.savepipe")),
		TimeDelay:      maxInt(req.TimeDelay, 0),
		TimeJitter:     maxInt(viper.GetInt("task.time_jitter_ms"), 0),
		RetryPasses:    maxInt(viper.GetInt("task.retry_passes"), 0),
		BufferSize:     firstPositive(viper.GetInt("task.mergebuf"), 32),
		OutputFormat:   viper.GetString("output.format"),
		MaxRetries:     firstPositive(viper.GetInt("task.max_retries"), 2),
		RequestTTL:     time.Duration(firstPositive(viper.GetInt("task.request_timeout_seconds"), 20)) * time.Second,
		RetryBackoff:   time.Duration(firstPositive(viper.GetInt("task.retry_backoff_ms"), 300)) * time.Millisecond,
		SlowBackoff:    time.Duration(firstPositive(viper.GetInt("task.slow_backoff_ms"), 1000)) * time.Millisecond,
		MaxSlowBackoff: time.Duration(firstPositive(viper.GetInt("task.max_slow_backoff_ms"), 10000)) * time.Millisecond,
		Policy:         policy,
	}

	task := NewTask(layers, tm, options)
	if task == nil {
		return nil, errors.New("failed to create task")
	}

	return task, nil
}

func newTileHTTPClient(timeout time.Duration, proxy string) *http.Client {
	proxyFunc := http.ProxyFromEnvironment
	if strings.TrimSpace(proxy) != "" {
		if parsed, err := urlpkg.Parse(strings.TrimSpace(proxy)); err == nil {
			proxyFunc = http.ProxyURL(parsed)
		}
	}

	transport := &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: -1,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     true,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   1,
		MaxConnsPerHost:       1,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (task *Task) setStatus(status TaskStatus) {
	task.mu.Lock()
	task.Status = status
	task.mu.Unlock()
	task.pauseCond.Broadcast()
}

func (task *Task) setError(err error) {
	if err == nil {
		return
	}
	task.mu.Lock()
	task.ErrorMessage = err.Error()
	task.mu.Unlock()
}

func (task *Task) snapshot() TaskResponse {
	task.mu.RLock()
	defer task.mu.RUnlock()

	startedAt := ""
	if task.StartedAt != nil {
		startedAt = task.StartedAt.Format(time.RFC3339)
	}

	finishedAt := ""
	if task.FinishedAt != nil {
		finishedAt = task.FinishedAt.Format(time.RFC3339)
	}

	progress := 0.0
	if task.Total > 0 {
		progress = float64(task.Current) / float64(task.Total)
	}

	return TaskResponse{
		ID:           task.ID,
		Name:         task.Name,
		File:         task.File,
		MinZoom:      task.Min,
		MaxZoom:      task.Max,
		Total:        task.Total,
		Current:      task.Current,
		Progress:     progress,
		Status:       string(task.Status),
		SuccessCount: task.SuccessCount,
		FailureCount: task.FailureCount,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		ErrorMessage: task.ErrorMessage,
	}
}

func (task *Task) Pause() error {
	task.mu.Lock()
	defer task.mu.Unlock()

	if task.Status != TaskRunning {
		return fmt.Errorf("task is not running")
	}

	task.Status = TaskPaused
	return nil
}

func (task *Task) Resume() error {
	task.mu.Lock()
	defer task.mu.Unlock()

	if task.Status != TaskPaused {
		return fmt.Errorf("task is not paused")
	}

	task.Status = TaskRunning
	task.pauseCond.Broadcast()
	return nil
}

func (task *Task) Cancel() error {
	task.mu.Lock()
	if task.Status == TaskCompleted || task.Status == TaskCancelled || task.Status == TaskFailed {
		task.mu.Unlock()
		return fmt.Errorf("task already finished")
	}
	task.Status = TaskCancelled
	task.mu.Unlock()

	task.cancel()
	task.pauseCond.Broadcast()
	return nil
}

func (task *Task) waitIfPaused() error {
	task.mu.Lock()
	defer task.mu.Unlock()

	for task.Status == TaskPaused {
		task.pauseCond.Wait()
	}

	if task.Status == TaskCancelled {
		return context.Canceled
	}

	return task.ctx.Err()
}

func (task *Task) markProcessed(success bool, err error) {
	task.mu.Lock()
	defer task.mu.Unlock()

	task.Current++
	if success {
		task.SuccessCount++
	} else {
		task.FailureCount++
		if err != nil && task.ErrorMessage == "" {
			task.ErrorMessage = err.Error()
		}
	}
}

func (task *Task) Bound() orb.Bound {
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, layer := range task.Layers {
		for _, g := range layer.Collection {
			bound = bound.Union(g.Bound())
		}
	}
	return bound
}

func (task *Task) Center() orb.Point {
	layer := task.Layers[len(task.Layers)-1]
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, g := range layer.Collection {
		bound = bound.Union(g.Bound())
	}
	return bound.Center()
}

func (task *Task) MetaItems() map[string]string {
	b := task.Bound()
	c := task.Center()
	return map[string]string{
		"id":          task.ID,
		"name":        task.Name,
		"description": task.Description,
		"attribution": `<a href="http://www.atlasdata.cn/" target="_blank">&copy; MapCloud</a>`,
		"basename":    task.TileMap.Name,
		"format":      task.TileMap.Format,
		"type":        task.TileMap.Schema,
		"pixel_scale": strconv.Itoa(TileSize),
		"version":     MBTileVersion,
		"bounds":      fmt.Sprintf(`%f,%f,%f,%f`, b.Left(), b.Bottom(), b.Right(), b.Top()),
		"center":      fmt.Sprintf(`%f,%f,%d`, c.X(), c.Y(), (task.Min+task.Max)/2),
		"minzoom":     strconv.Itoa(task.Min),
		"maxzoom":     strconv.Itoa(task.Max),
		"json":        task.TileMap.JSON,
	}
}

func (task *Task) SetupMBTileTables() error {
	if task.File == "" {
		outdir := viper.GetString("output.directory")
		if err := os.MkdirAll(outdir, os.ModePerm); err != nil {
			return err
		}
		task.File = filepath.Join(outdir, fmt.Sprintf("%s-z%d-%d.%s.mbtiles", task.Name, task.Min, task.Max, task.ID))
	}

	_ = os.Remove(task.File)

	db, err := sql.Open("sqlite", task.File)
	if err != nil {
		return err
	}

	if err := optimizeConnection(db); err != nil {
		_ = db.Close()
		return err
	}

	statements := []string{
		"create table if not exists tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob);",
		"create table if not exists metadata (name text, value text);",
		"create unique index if not exists name on metadata (name);",
		"create unique index if not exists tile_index on tiles(zoom_level, tile_column, tile_row);",
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return err
		}
	}

	for name, value := range task.MetaItems() {
		if _, err := db.Exec("insert or replace into metadata (name, value) values (?, ?)", name, value); err != nil {
			_ = db.Close()
			return err
		}
	}

	task.db = db
	return nil
}

func (task *Task) setupOutput() error {
	if task.outformat == "mbtiles" {
		return task.SetupMBTileTables()
	}

	if task.File == "" {
		outdir := viper.GetString("output.directory")
		task.File = filepath.Join(outdir, fmt.Sprintf("%s-z%d-%d.%s", task.Name, task.Min, task.Max, task.ID))
	}

	return os.MkdirAll(task.File, os.ModePerm)
}

func (task *Task) closeOutput() error {
	if task.db == nil {
		return nil
	}

	if err := optimizeDatabase(task.db); err != nil {
		log.Warnf("optimize mbtiles failed for task %s: %v", task.ID, err)
	}

	err := task.db.Close()
	task.db = nil
	return err
}

func (task *Task) runFetchers() {
	for i := 0; i < task.workerCount; i++ {
		task.tileWG.Add(1)
		go func() {
			defer task.tileWG.Done()
			for job := range task.jobs {
				if err := task.waitIfPaused(); err != nil {
					return
				}
				if err := task.processTile(job); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					if task.scheduleRetry(job, err) {
						task.recordTileFailure(job, err, true)
						continue
					}
					task.recordTileFailure(job, err, false)
					task.markProcessed(false, err)
				}
			}
		}()
	}
}

func (task *Task) runSavers() {
	for i := 0; i < task.savePipeSize; i++ {
		task.saveWG.Add(1)
		go func() {
			defer task.saveWG.Done()
			for tile := range task.savingpipe {
				if err := task.waitIfPaused(); err != nil {
					return
				}
				if err := task.saveTile(tile); err != nil {
					task.markProcessed(false, err)
					log.Errorf("save %v failed: %v", tile.T, err)
					continue
				}
				task.markProcessed(true, nil)
			}
		}()
	}
}

func (task *Task) processTile(job TileJob) error {
	if err := task.waitForRequestWindow(); err != nil {
		return err
	}

	body, err := task.fetchTile(job.Tile, job.URL)
	if err != nil {
		task.recordThrottle(err)
		return err
	}

	task.recordSuccess()

	if err := validateTileResponse(body, task.TileMap.Format); err != nil {
		return err
	}

	body, err = normalizeTileData(body, task.TileMap.Format)
	if err != nil {
		return err
	}

	td := Tile{T: job.Tile, C: body}
	if task.TileMap.Format == PBF {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			_ = zw.Close()
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		td.C = buf.Bytes()
	}

	select {
	case <-task.ctx.Done():
		return task.ctx.Err()
	case task.savingpipe <- td:
		return nil
	}
}

func (task *Task) waitForRequestWindow() error {
	delay := time.Duration(task.timeDelay) * time.Millisecond
	if task.timeJitter > 0 {
		delay += time.Duration(rand.Intn(task.timeJitter+1)) * time.Millisecond
	}

	task.mu.RLock()
	throttleUntil := task.throttleUntil
	task.mu.RUnlock()
	if !throttleUntil.IsZero() {
		if extra := time.Until(throttleUntil); extra > 0 {
			delay += extra
		}
	}

	if delay > 0 {
		select {
		case <-task.ctx.Done():
			return task.ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil
}

func (task *Task) scheduleRetry(job TileJob, err error) bool {
	if job.Pass >= task.retryPasses {
		return false
	}
	if !isRetryable(err) {
		return false
	}

	task.retryMu.Lock()
	task.retryQueue = append(task.retryQueue, TileJob{
		Tile: job.Tile,
		URL:  job.URL,
		Pass: job.Pass + 1,
	})
	task.retryMu.Unlock()
	return true
}

func (task *Task) recordTileFailure(job TileJob, err error, retryable bool) {
	if err == nil {
		return
	}
	record := TileFailureRecord{
		Z:            int(job.Tile.Z),
		X:            int(job.Tile.X),
		Y:            int(job.Tile.Y),
		URL:          prepareTileURL(job.Tile, job.URL),
		ErrorMessage: err.Error(),
		Retryable:    retryable,
		Attempt:      job.Pass + 1,
		CreatedAt:    time.Now(),
	}
	task.failureMu.Lock()
	task.failureRecords = append(task.failureRecords, record)
	task.failureMu.Unlock()
}

func (task *Task) FailureRecords() []TileFailureRecord {
	task.failureMu.Lock()
	defer task.failureMu.Unlock()
	records := make([]TileFailureRecord, len(task.failureRecords))
	copy(records, task.failureRecords)
	return records
}

func (task *Task) nextRetryBatch() []TileJob {
	task.retryMu.Lock()
	defer task.retryMu.Unlock()

	if len(task.retryQueue) == 0 {
		return nil
	}

	batch := make([]TileJob, len(task.retryQueue))
	copy(batch, task.retryQueue)
	task.retryQueue = nil
	return batch
}

func (task *Task) recordThrottle(err error) {
	statusCode := extractHTTPStatusCode(err)
	category := fetchErrorCategory(err)
	if !isThrottleStatus(statusCode) && category != FetchErrorBlocked && category != FetchErrorThrottle {
		return
	}

	level := 1
	until := time.Now().Add(task.slowBackoff)

	task.mu.Lock()
	if task.throttleLevel > 0 {
		level = task.throttleLevel + 1
	}
	if level > 5 {
		level = 5
	}
	task.throttleLevel = level
	backoff := scaledSlowBackoff(task.slowBackoff, task.maxSlowBackoff, level)
	until = time.Now().Add(backoff)
	task.throttleUntil = until
	task.mu.Unlock()

	log.Warnf("throttle detected for task %s (%s/%d), level=%d, backing off for %s", task.ID, category, statusCode, level, backoff)
}

func (task *Task) recordSuccess() {
	task.mu.Lock()
	defer task.mu.Unlock()

	if task.throttleLevel > 0 {
		task.throttleLevel--
	}
	if task.throttleLevel == 0 {
		task.throttleUntil = time.Time{}
		return
	}
	if time.Now().After(task.throttleUntil) {
		task.throttleUntil = time.Time{}
	}
}

func (task *Task) clientForProxy(proxy string) *http.Client {
	key := strings.TrimSpace(proxy)
	task.clientMu.Lock()
	defer task.clientMu.Unlock()

	client, exists := task.clientCache[key]
	if exists {
		return client
	}

	client = newTileHTTPClient(task.requestTTL, key)
	task.clientCache[key] = client
	return client
}

func (task *Task) nextProxy() string {
	if task.proxyRotator == nil {
		return ""
	}
	return task.proxyRotator.Next()
}

func (task *Task) applyRequestHeaders(req *http.Request) {
	req.Close = true
	if strings.TrimSpace(task.policy.UserAgent) != "" {
		req.Header.Set("User-Agent", task.policy.UserAgent)
	}
	if strings.TrimSpace(task.policy.Referer) != "" {
		req.Header.Set("Referer", task.policy.Referer)
	}
	if task.policy.Headers != nil {
		for key, value := range task.policy.Headers {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			req.Header.Set(key, value)
		}
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "close")
	}
}

func (task *Task) fetchTile(mt maptile.Tile, url string) ([]byte, error) {
	tileURL := prepareTileURL(mt, url)
	if task.policy.RotateHosts {
		tileURL = distributeTianDiTuHost(mt, tileURL)
	}
	var lastErr error
	var lastProxy string

	for attempt := 0; attempt <= task.maxRetries; attempt++ {
		if err := task.ctx.Err(); err != nil {
			return nil, err
		}

		lastProxy = task.nextProxy()

		req, err := http.NewRequestWithContext(task.ctx, http.MethodGet, tileURL, nil)
		if err != nil {
			return nil, err
		}
		task.applyRequestHeaders(req)

		start := time.Now()
		resp, err := task.clientForProxy(lastProxy).Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode != http.StatusOK {
				lastErr = HTTPStatusError{StatusCode: resp.StatusCode}
			} else if len(body) == 0 {
				lastErr = errors.New("empty tile response")
			} else {
				cost := time.Since(start).Milliseconds()
				log.Infof("tile(z:%d, x:%d, y:%d), %dms , %.2f kb, %s", mt.Z, mt.X, mt.Y, cost, float32(len(body))/1024.0, tileURL)
				return body, nil
			}
		}

		if !isRetryable(lastErr) || attempt == task.maxRetries {
			break
		}

		backoff := task.retryBackoff * time.Duration(attempt+1)
		select {
		case <-task.ctx.Done():
			return nil, task.ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, classifyFetchFailure(tileURL, lastProxy, lastErr)
}

func (task *Task) saveTile(tile Tile) error {
	if task.outformat == "mbtiles" {
		return saveToMBTile(tile, task.db)
	}
	return saveToFiles(tile, task)
}

func (task *Task) enqueueTiles() error {
	for _, layer := range task.Layers {
		if err := task.waitIfPaused(); err != nil {
			return err
		}

		bar := pb.New64(layer.Count).Prefix(fmt.Sprintf("Zoom %d : ", layer.Zoom)).Postfix("\n")
		bar.Start()

		tilelist := make(chan maptile.Tile, task.bufSize)
		go tilecover.CollectionChannel(layer.Collection, maptile.Zoom(layer.Zoom), tilelist)

		for tile := range tilelist {
			if err := task.waitIfPaused(); err != nil {
				bar.Finish()
				return err
			}
			select {
			case <-task.ctx.Done():
				bar.Finish()
				return task.ctx.Err()
			case task.jobs <- TileJob{Tile: tile, URL: layer.URL, Pass: 0}:
				bar.Increment()
				if task.Bar != nil {
					task.Bar.Increment()
				}
			}
		}

		bar.FinishPrint(fmt.Sprintf("Task %s Zoom %d queued", task.ID, layer.Zoom))
	}

	return nil
}

func (task *Task) enqueueRetryBatch(retryJobs []TileJob) error {
	if len(retryJobs) == 0 {
		return nil
	}

	log.Warnf("task %s entering retry pass with %d tiles", task.ID, len(retryJobs))
	for _, job := range retryJobs {
		if err := task.waitIfPaused(); err != nil {
			return err
		}
		select {
		case <-task.ctx.Done():
			return task.ctx.Err()
		case task.jobs <- job:
		}
	}

	return nil
}

func (task *Task) Run() {
	started := time.Now()
	task.mu.Lock()
	task.StartedAt = &started
	task.Status = TaskRunning
	task.mu.Unlock()

	task.Bar = pb.New64(task.Total).Prefix("Task : ").Postfix("\n")
	task.Bar.Start()

	if err := task.setupOutput(); err != nil {
		task.fail(err)
		return
	}

	task.runSavers()
	enqueueErr := task.runFetchPhase(task.enqueueTiles)
	retryJobs := task.nextRetryBatch()
	for enqueueErr == nil && len(retryJobs) > 0 {
		batch := retryJobs
		enqueueErr = task.runFetchPhase(func() error {
			return task.enqueueRetryBatch(batch)
		})
		if enqueueErr != nil {
			break
		}
		retryJobs = task.nextRetryBatch()
	}
	close(task.savingpipe)
	task.saveWG.Wait()
	if task.Bar != nil {
		task.Bar.Finish()
	}

	closeErr := task.closeOutput()

	switch {
	case errors.Is(enqueueErr, context.Canceled):
		task.finish(TaskCancelled, nil)
	case task.Status == TaskCancelled:
		task.finish(TaskCancelled, nil)
	case enqueueErr != nil:
		task.fail(enqueueErr)
	case closeErr != nil:
		task.fail(closeErr)
	case task.FailureCount > 0 && task.SuccessCount == 0:
		task.fail(errors.New("task finished with no successful tiles"))
	case task.Status != TaskCancelled:
		task.finish(TaskCompleted, nil)
	}
}

func (task *Task) runFetchPhase(enqueue func() error) error {
	task.jobs = make(chan TileJob, task.bufSize)
	task.runFetchers()
	err := enqueue()
	close(task.jobs)
	task.tileWG.Wait()
	return err
}

func (task *Task) fail(err error) {
	task.setError(err)
	task.finish(TaskFailed, err)
}

func (task *Task) finish(status TaskStatus, err error) {
	finished := time.Now()
	task.mu.Lock()
	task.Status = status
	if err != nil && task.ErrorMessage == "" {
		task.ErrorMessage = err.Error()
	}
	task.FinishedAt = &finished
	task.mu.Unlock()
	task.pauseCond.Broadcast()
	task.cancel()
}

func prepareTileURL(t maptile.Tile, url string) string {
	url = strings.ReplaceAll(url, "{x}", strconv.Itoa(int(t.X)))
	url = strings.ReplaceAll(url, "{y}", strconv.Itoa(int(t.Y)))
	maxY := int(math.Pow(2, float64(t.Z))) - 1
	url = strings.ReplaceAll(url, "{-y}", strconv.Itoa(maxY-int(t.Y)))
	url = strings.ReplaceAll(url, "{z}", strconv.Itoa(int(t.Z)))
	return url
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	if failure := extractFetchFailure(err); failure != nil {
		switch failure.Category {
		case FetchErrorThrottle, FetchErrorBlocked, FetchErrorUpstream, FetchErrorProxy, FetchErrorNetwork:
			return true
		case FetchErrorContent:
			return failure.StatusCode >= http.StatusInternalServerError || failure.StatusCode == http.StatusTooManyRequests || failure.StatusCode == http.StatusTeapot
		default:
			if failure.StatusCode > 0 {
				return failure.StatusCode >= http.StatusInternalServerError
			}
		}
	}

	var statusErr HTTPStatusError
	if errors.As(err, &statusErr) {
		return isThrottleStatus(statusErr.StatusCode) || statusErr.StatusCode >= http.StatusInternalServerError
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

func isThrottleStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTeapot, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func scaledSlowBackoff(base time.Duration, max time.Duration, level int) time.Duration {
	if base <= 0 {
		return 0
	}
	if level < 1 {
		level = 1
	}
	backoff := base
	for i := 1; i < level; i++ {
		backoff *= 2
		if max > 0 && backoff >= max {
			return max
		}
	}
	if max > 0 && backoff > max {
		return max
	}
	return backoff
}

func distributeTianDiTuHost(tile maptile.Tile, rawURL string) string {
	parsed, err := urlpkg.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".tianditu.gov.cn") {
		return rawURL
	}

	shard := rand.Intn(8)
	parsed.Host = strings.Replace(parsed.Host, parsed.Hostname(), fmt.Sprintf("t%d.tianditu.gov.cn", shard), 1)
	return parsed.String()
}

func maxInt(v int, minimum int) int {
	if v < minimum {
		return minimum
	}
	return v
}

func maxDuration(v time.Duration, minimum time.Duration) time.Duration {
	if v < minimum {
		return minimum
	}
	return v
}
