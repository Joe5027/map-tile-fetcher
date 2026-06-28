package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/shiena/ansicolor"
	log "github.com/sirupsen/logrus"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/spf13/viper"
	_ "modernc.org/sqlite"
)

// flag
var (
	hf                 bool
	cf                 string
	workerTaskRecordID string
	workerRunID        string
)

func init() {
	flag.BoolVar(&hf, "h", false, "this help")
	flag.StringVar(&cf, "c", "conf.toml", "set config `file`")
	flag.StringVar(&workerTaskRecordID, "worker-task-record-id", "", "run a child task record in worker mode")
	flag.StringVar(&workerTaskRecordID, "worker-plan-id", "", "legacy alias for -worker-task-record-id")
	flag.StringVar(&workerRunID, "worker-run-id", "", "run a task run in worker mode")
	// 改变默认的 Usage，flag包中的Usage 其实是一个函数类型。这里是覆盖默认函数实现，具体见后面Usage部分的分析
	flag.Usage = usage
	//InitLog 初始化日志
	log.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		ShowFullLevel:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		// FieldsOrder: []string{"component", "category"},
	})
	// then wrap the log output with it
	log.SetOutput(ansicolor.NewAnsiColorWriter(os.Stdout))
	log.SetLevel(log.DebugLevel)
}
func usage() {
	fmt.Fprintf(os.Stderr, `tiler version: tiler/v0.1.0
Usage: tiler [-h] [-c filename]
`)
	flag.PrintDefaults()
}

// initConf 初始化配置
func initConf(cfgFile string) {
	configExists := true
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		configExists = false
		log.Warnf("config file(%s) not exist", cfgFile)
	}
	viper.SetConfigType("toml")
	viper.SetConfigFile(cfgFile)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv() // read in environment variables that match
	err := viper.ReadInConfig()
	if err != nil {
		if configExists {
			log.Fatalf("read config file(%s) error, details: %s", viper.ConfigFileUsed(), err)
		}
		log.Warnf("read config file(%s) error, details: %s", viper.ConfigFileUsed(), err)
	}
	viper.SetDefault("app.version", "v 0.1.0")
	viper.SetDefault("app.title", "MapCloud Tiler")
	viper.SetDefault("app.port", "8081")
	viper.SetDefault("app.database", "tiler.db")
	viper.SetDefault("output.format", "mbtiles")
	viper.SetDefault("output.directory", "output")
	viper.SetDefault("task.workers", 1)
	viper.SetDefault("task.savepipe", 1)
	viper.SetDefault("task.timedelay", 200)
	viper.SetDefault("task.time_jitter_ms", 150)
	viper.SetDefault("task.retry_passes", 1)
	viper.SetDefault("task.mergebuf", 32)
	viper.SetDefault("task.max_retries", 2)
	viper.SetDefault("task.request_timeout_seconds", 20)
	viper.SetDefault("task.retry_backoff_ms", 300)
	viper.SetDefault("task.slow_backoff_ms", 1000)
	viper.SetDefault("task.max_slow_backoff_ms", 10000)
	viper.SetDefault("auth.enabled", true)
	viper.SetDefault("auth.default_username", "admin")
	viper.SetDefault("auth.default_password", "adminmap")
}

func main() {
	flag.Parse()
	if hf {
		flag.Usage()
		return
	}

	if cf == "" {
		cf = "conf.toml"
	}
	initConf(cf)

	// 初始化数据库
	initDB()

	if shouldRunWorkerMode() {
		if err := runWorkerProcess(workerTaskRecordID, workerRunID); err != nil {
			log.Fatalf("worker process failed: %v", err)
		}
		return
	}

	// 初始化静态文件目录
	ensureStaticDir()

	// 启动Web服务器
	initServer()
}

// 确保静态文件目录存在
func ensureStaticDir() {
	if _, err := os.Stat("./static"); os.IsNotExist(err) {
		os.MkdirAll("./static", os.ModePerm)
	}
}

func shouldRunWorkerMode() bool {
	return strings.TrimSpace(workerTaskRecordID) != "" && strings.TrimSpace(workerRunID) != ""
}
