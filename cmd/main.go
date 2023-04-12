package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/signalsciences/waf-testing-framework/pkg/app"
	"github.com/signalsciences/waf-testing-framework/pkg/config"
	"github.com/signalsciences/waf-testing-framework/pkg/logs"
	"github.com/signalsciences/waf-testing-framework/pkg/results"
	"github.com/sirupsen/logrus"
)

//version will be overwritten by release process flag
var waftfversion = "0000.00.0"

func main() {
	//the config file flag
	var configFile string
	var debugMode, version bool
	var workerLimit, maxProcs, ratelimit int
	flag.StringVar(&configFile, "config", "./config.yml", "path to the yaml config file")
	flag.StringVar(&configFile, "c", "./config.yml", "path to the yaml config file (shorthand)")
	flag.IntVar(&workerLimit, "worker", 10, "set the maximum number of requests to be sent concurrently")
	flag.IntVar(&workerLimit, "w", 10, "set the maximum number of requests to be sent concurrently (shorthand")
	flag.IntVar(&maxProcs, "processor", runtime.NumCPU(), "The maximum number of operating system threads (CPUs) that will be used to execute the testing tool simultaneously")
	flag.IntVar(&maxProcs, "p", runtime.NumCPU(), "The maximum number of operating system threads (CPUs) that will be used to execute the testing tool simultaneously (shorthand)")
	flag.BoolVar(&debugMode, "debug", false, "sets the log level to debug")
	flag.BoolVar(&debugMode, "d", false, "sets the log level to debug (shorthand)")
	flag.BoolVar(&version, "version", false, "prints the version of WAF testing tool")
	flag.BoolVar(&version, "v", false, "prints the version of WAF testing tool (shorthand)")
	flag.IntVar(&ratelimit, "rate", 50, "set the maximum transatcions per second WTT will generate")
	flag.IntVar(&ratelimit, "r", 50, "set the maximum transatcions per second WTT will generate (shorthand)")
	flag.Parse()

	// print the version and exit
	if version {
		fmt.Printf("WAF Testing Framework version %v\n", waftfversion)
		os.Exit(0)
	}
	//set environment parameters
	runtime.GOMAXPROCS(maxProcs)
	if ratelimit <= 0 {
		ratelimit = 50
	}
	rate := time.Second / time.Duration(ratelimit)
	//init new log
	log := logs.NewLogger(filepath.FromSlash("output/runtime.log"))
	if debugMode {
		log.SetLevel(logrus.DebugLevel)
	}
	errorLog := logs.NewLogger(filepath.FromSlash("output/error.log"))
	errorLog.SetLevel(logrus.ErrorLevel)
	log.Printf("starting WAF Testing Framework version %v\n", waftfversion)
	log.Printf("using %v CPUs and %v workers", maxProcs, workerLimit)

	//read in the provided file
	r, err := os.Open(configFile)
	if err != nil {
		fmt.Printf("unable to read yaml file: %v", err)
		log.Fatalf("unable to read yaml file: %v", err)
	}
	defer r.Close()

	//load test configs
	yamlTests, err := config.ParseYamlFile(r)
	if err != nil {
		fmt.Printf("unable to test configs: %v", err)
		log.Fatalf("unable to test configs: %v", err)
	}

	//parse the configs
	testRun, err := config.ParseConfigs(yamlTests)
	if err != nil {
		fmt.Printf("unable to parse configs: %v", err)
		log.Fatalf("unable to parse configs: %v", err)
	}

	//channel to put tests on
	testsChan := make(chan *app.TestRequest, 50)
	//channel to put results on
	resultsChan := make(chan *results.TestResult, 50)
	//channel to indicate queuing is done
	doneQueuingChan := make(chan struct{}, 1)
	//channel to indicate processing is done
	doneProcessingChan := make(chan struct{}, 1)
	//the stop channel stops the workers.
	stopChan := make(chan struct{})
	//the rate limit throttle channel
	rateLimiter := time.NewTicker(rate)
	//initialize application object
	a := &app.Application{
		Client: &http.Client{
			Timeout: time.Second * 10,
		},
		TestRun:            testRun,
		TestsChan:          testsChan,
		ResultsChan:        resultsChan,
		StopChan:           stopChan,
		Log:                log,
		ErrorLog:           errorLog,
		Results:            results.InitResults(testRun),
		DoneQueuingChan:    doneQueuingChan,
		DoneProcessingChan: doneProcessingChan,
		RateLimiter:        rateLimiter,
		WorkerLimit:        workerLimit,
	}
	//ensure we can reach the targeted locations
	a.ValidateURI()
	//create a listener in a goroutine which will notify
	//the done channel when it receives an interrupt from the OS.
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	//cleanup function to stop receiving signals and call cancel()
	defer func() {
		signal.Stop(interrupt)
		cancel()
	}()
	go func() {
		select {
		case <-interrupt:
			fmt.Println("\r- Interrupt signal detected - shutting down workers")
			a.Log.Infoln("interrupt signal detected - shutting down workers")
			close(a.StopChan)
			a.RequestWG.Wait()
			a.ResultWG.Wait()
			fmt.Println(("all workers shut down"))
			a.Log.Infoln("all workers shut down")
			os.Exit(0)
		case <-ctx.Done():
			fmt.Println("finished")
			a.Log.Infoln("finished")
		}
	}()
	//run the app
	a.Run()
	fmt.Println("Generating report....")
	//calculate counters and process data for the report
	a.Results.ProcessResults()
	a.Results.ReportData()
	//generate reports
	if err := a.Results.GenerateReports(); err != nil {
		fmt.Printf("Unable to generate report: %v", err)
		log.Fatalf("Unable to generate report: %v", err)
	}
}
