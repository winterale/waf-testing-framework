package app

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/signalsciences/waf-testing-framework/pkg/config"
	"github.com/signalsciences/waf-testing-framework/pkg/results"
	"github.com/sirupsen/logrus"
)

const (
	stringFN   string = "falseNegative"
	stringFP   string = "falsePositive"
	stringInv  string = "invalid"
	stringErr  string = "error"
	stringPass string = "pass"
)

var resultMapMutext = sync.RWMutex{}

//HTTPClient wraps the http.Client to allow for setting timeouts
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

//Application represents the application object
type Application struct {
	Client             HTTPClient
	Log                *logrus.Logger
	ErrorLog           *logrus.Logger
	TestRun            *config.TestRun
	TestsChan          chan *TestRequest
	ResultsChan        chan *results.TestResult
	StopChan           chan struct{}
	Results            *results.Results
	DoneQueuingChan    chan struct{}
	DoneProcessingChan chan struct{}
	RateLimiter        *time.Ticker
	WorkerLimit        int
	RequestWG          sync.WaitGroup
	ResultWG           sync.WaitGroup
}

//TestRequest represents a single test to be run against the app
type TestRequest struct {
	FileName     string
	SetName      string
	Line         int
	Payload      string
	CheckPayload string
	TestType     string
	Outcome      string
	Location     string
	AllowCon     *config.Condition
	BlockCon     *config.Condition
	Request      *http.Request
	Response     *http.Response
	Error        error
}

//ValidateURI loops through all the configured test URIs to ensure they are of valid format and reachable
func (a *Application) ValidateURI() {
	for _, testSet := range a.TestRun.TestSets {
		u, err := url.ParseRequestURI(testSet.URI)
		if err != nil {
			fmt.Printf("Exiting because invalid URI %s. See log for details\n", testSet.URI)
			a.Log.Fatalf("URI %s invalid, error: %v\n", testSet.URI, err)
		}
		timeout := 1 * time.Second
		_, err = net.DialTimeout("tcp", u.Hostname()+":"+u.Port(), timeout)
		if err != nil {
			fmt.Printf("Exiting because URI %s unreachable. See log for details\n", u.Hostname()+":"+u.Port())
			a.Log.Fatalf("URI %s unreachable, error: %v\n", u.Hostname()+":"+u.Port(), err)
		}
	}
}

//Run spawns request workers and result workers up to the worker limit defined in the test configurations,
//enqueues testRequest objects for processing, and exits when all workers are finished.
func (a *Application) Run() {
	fmt.Println("begin processing...")
	a.Log.Infoln("begin processing...")

	//use waitgroups to know when all work has been completed
	//spawn workers
	for w := 1; w <= a.WorkerLimit; w++ {
		a.RequestWG.Add(1)
		go a.requestWorker(w, a.StopChan)
	}
	for r := 1; r <= a.WorkerLimit; r++ {
		a.ResultWG.Add(1)
		go a.resultWorker(r, a.StopChan)
	}

	//add testRequests to the channel
	a.queueTests()

	//wait for all request processing to finish
	a.RequestWG.Wait()

	//send the signal that no new requests will be queued by closing
	//the a.DoneProcessingChan channel
	close(a.DoneProcessingChan)
	fmt.Println("finished sending requests")
	a.Log.Infoln("finished sending requests")
	close(a.TestsChan)

	//wait for all result processing to finish
	a.ResultWG.Wait()
	fmt.Println("finished processing results")
	a.Log.Infoln("finished processing results")
	close(a.ResultsChan)
	a.RateLimiter.Stop()
}

//requestWorker is a worker that will read testRequest objects from the a.TestChan channel,
//run them against the target URI, and then enqueue results onto the a.ResultChan channel
func (a *Application) requestWorker(id int, stopChan <-chan struct{}) {
	//defer shutting down the worker
	defer func() {
		a.Log.Debugf("Shutting down request worker %v\n", id)
		a.RequestWG.Done()
	}()
	for {
		select {
		//handle an interrupt such as ctrl+c
		case <-stopChan:
			return
		//when all tests have been queued this signal channel will close
		case <-a.DoneQueuingChan:
			//wait for the a.TestChan to be drained before stopping the worker
			if len(a.TestsChan) == 0 {
				return
			}
		//process a request from the a.TestChan channel
		case testRequest := <-a.TestsChan:
			setName := testRequest.SetName
			fileName := testRequest.FileName
			location := testRequest.Location
			a.Log.Debugf("request worker %v processing payload %v from line %v in file %v against location %v\n", id, testRequest.Payload, testRequest.Line, testRequest.FileName, testRequest.Location)
			//create the testResult
			testResult := &results.TestResult{
				SetName:  setName,
				FileName: fileName,
				Line:     testRequest.Line,
				Payload:  testRequest.Payload,
				Location: location,
			}
			//check for invalid requests before sending
			invalid, illegalChars, _ := checkInvalidChars(testRequest.CheckPayload, testRequest.Location)
			if invalid {
				testResult.Outcome = stringInv
				testResult.Response = illegalChars
				testResult.Request = fmt.Sprintf("Invalid payload for location: %v", testRequest.CheckPayload)
				a.ResultsChan <- testResult
				continue
			}
			//save the request body for reporting if it isn't nil
			var reqBody io.ReadCloser
			if testRequest.Request.Body != nil {
				reqBody, _ = testRequest.Request.GetBody()
			}
			<-a.RateLimiter.C
			resp, err := a.Client.Do(testRequest.Request)
			//if there is an error transacting the request, save the error and
			//push the invalid result to a.ResultsChan
			if err != nil {
				//print to runtime.log if log level is debug
				a.ErrorLog.WithFields(logrus.Fields{
					"File":     testRequest.FileName,
					"Line":     testRequest.Line,
					"Payload":  testRequest.Payload,
					"Location": testRequest.Location,
				}).Errorf("transaction error: %v\n", err)
				testResult.Outcome = stringErr
				testResult.Request = err.Error()
				a.ResultsChan <- testResult
				continue
			}
			testRequest.Response = resp
			//restore request body after Do() drains it
			testRequest.Request.Body = reqBody
			//get outcome to see if test passed or not
			testOutcome, err := getOutcome(testRequest)
			if err != nil {
				resp.Body.Close()
				testRequest.Response.Body.Close()
				a.ErrorLog.WithFields(logrus.Fields{
					"File":     testRequest.FileName,
					"Line":     testRequest.Line,
					"Payload":  testRequest.Payload,
					"Location": testRequest.Location,
					"Outcome":  testOutcome,
				}).Error(err)
				testResult.Outcome = stringErr
				testResult.Request = err.Error()
				testResult.Response = ""
				a.ResultsChan <- testResult
				continue
			}
			//record results of all non-passed tests
			if testOutcome != stringPass {
				testResult.Outcome = testOutcome
				//get request body
				request, err := httputil.DumpRequestOut(testRequest.Request, true)
				if err != nil {
					resp.Body.Close()
					testRequest.Response.Body.Close()
					a.ErrorLog.WithFields(logrus.Fields{
						"File":     testRequest.FileName,
						"Line":     testRequest.Line,
						"Payload":  testRequest.Payload,
						"Location": testRequest.Location,
					}).Errorf("can't dump request: %v\n", err)
					testResult.Outcome = stringErr
					testResult.Request = err.Error()
					testResult.Response = ""
					a.ResultsChan <- testResult
					continue
				}
				testResult.Request = string(request)
				//get response body
				response, err := httputil.DumpResponse(resp, false)
				resp.Body.Close()
				testRequest.Response.Body.Close()
				if err != nil {
					a.ErrorLog.WithFields(logrus.Fields{
						"File":     testRequest.FileName,
						"Line":     testRequest.Line,
						"Payload":  testRequest.Payload,
						"Location": testRequest.Location,
					}).Errorf("can't dump response: %v\n", err)
					testResult.Response = ""
					testResult.Outcome = stringErr
					a.ResultsChan <- testResult
					continue
				}

				testResult.Response = string(response)
				a.ResultsChan <- testResult
			}
			//increment the total test count
			resultMapMutext.Lock()
			if testRequest.TestType == "falsePositive" {
				a.Results.SetCounts[setName].TotalFPTestCount++
			}
			if testRequest.TestType == "falseNegative" {
				a.Results.SetCounts[setName].TotalFNTestCount++
			}
			resultMapMutext.Unlock()
			a.Log.Debugf("request worker %v done\n", id)
		}
	}
}

//resultWorker is a worker that will read testResult objects from the a.ResultsChan channel,
//and add them to the a.Results object
func (a *Application) resultWorker(id int, stopChan <-chan struct{}) {
	//defer shutting down the worker
	defer func() {
		a.Log.Debugf("Shutting down result worker %v\n", id)
		a.ResultWG.Done()
	}()
	for {
		select {
		//handle an interrupt such as ctrl+c
		case <-stopChan:
			return
		//when all tests have run and been pushed onto the
		//a.ResultsChan this signal cannel will close
		case <-a.DoneProcessingChan:
			//channel has been drained
			if len(a.ResultsChan) == 0 {
				return
			}
		//process a result
		case testResult := <-a.ResultsChan:
			a.Log.Debugf("result worker %v processing %v result %v from line %v in file %v against location %v\n", id, testResult.Outcome, testResult.Payload, testResult.Line, testResult.FileName, testResult.Location)
			setName := testResult.SetName
			fileName := testResult.FileName
			location := testResult.Location
			line := testResult.Line
			//only save tests that dont have a "pass" outcome
			if testResult.Outcome != stringPass {
				resultMapMutext.Lock()
				//create the maps if first time saving to one
				if a.Results.FileResults[fileName].PayloadResults[line] == nil {
					a.Results.FileResults[fileName].PayloadResults[line] = &results.PayloadResult{
						Line:       line,
						Payload:    testResult.Payload,
						SetResults: make(map[string]*results.SetResult),
					}
				}
				if a.Results.FileResults[fileName].PayloadResults[line].SetResults[setName] == nil {
					a.Results.FileResults[fileName].PayloadResults[line].SetResults[setName] = &results.SetResult{
						Locations: make(map[string]*results.TestResult),
					}
				}
				if a.Results.FileResults[fileName].PayloadResults[line].SetResults[setName].Locations[location] == nil {
					a.Results.FileResults[fileName].PayloadResults[line].SetResults[setName].Locations[location] = &results.TestResult{}
				}
				if !intContains(a.Results.FileResults[fileName].FailedLines, line) {
					a.Results.FileResults[fileName].FailedLines = append(a.Results.FileResults[fileName].FailedLines, line)
				}
				//save the result
				a.Results.FileResults[fileName].PayloadResults[line].SetResults[setName].Locations[location] = testResult
				//increment the correct counter
				switch testResult.Outcome {
				case stringFN:
					a.Results.SetCounts[setName].FnCount++
				case stringFP:
					a.Results.SetCounts[setName].FpCount++
				case stringInv:
					a.Results.SetCounts[setName].InvCount++
				case stringErr:
					a.Results.SetCounts[setName].ErrCount++
				}
				resultMapMutext.Unlock()
				a.Log.Debugf("result worker %v done\n", id)
			}
		}
	}
}

//queueTests reads through all the payload files for the entire test run and creates and enqueues
//testRequest objects that will be used to run individual tests against the application
func (a *Application) queueTests() {
	testRun := a.TestRun
	//for each file
	for _, file := range testRun.TestFiles {
		fmt.Printf("processsing %v...\n", file.File)
		a.Log.Infof("processsing %v...\n", file.File)
		payloadFile, err := os.Open(file.File)
		if err != nil {
			fmt.Println("error running tests. Check log for details")
			a.Log.Fatalf("unable to open file %v: %v", file.File, err)
		}
		scanner := bufio.NewScanner(payloadFile)
		line := 1

		bar := progressbar.NewOptions(-1,
			progressbar.OptionSetDescription("processing lines..."),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionShowCount())
		//for each line in the file
		for scanner.Scan() {
			bar.Add(1)
			//for each testSet
			for _, testSet := range a.TestRun.TestSets {
				//for each location specified by the configurations
				for _, location := range testRun.Locations {
					parts := strings.Split(file.File, string(os.PathSeparator))
					parentDir := parts[len(parts)-2]
					//build testRequest object
					testRequest := &TestRequest{
						SetName:  testSet.Name,
						Location: location.Location,
						FileName: parentDir + string(os.PathSeparator) + filepath.Base(file.File),
						TestType: file.TestType,
						Line:     line,
						Payload:  scanner.Text(),
						AllowCon: testSet.AllowCondition,
						BlockCon: testSet.BlockCondition,
					}
					//adjust the request to include the payload in the correct location
					err = a.buildRequest(testRequest, location, testSet)
					if err != nil {
						payloadFile.Close()
						fmt.Println("Error building request. Check log for details")
						a.Log.Fatalf("unable to build request: %v", err)
					}
					//place the request on the a.TestsChan channel
					a.TestsChan <- testRequest
				}
			}
			line++
		}
		//all lines of the file have been processed
		payloadFile.Close()
		fmt.Println("finished")
		a.Log.Infof("finished processsing %v\n", file.File)
	}
	close(a.DoneQueuingChan)
	//all tests are done being written to a.TestsChan, but we can't close it because reads could still be going on
	fmt.Println("finished queuing tests")
	a.Log.Infof("finished queuing tests")
}

//getOutcome looks to see if the response received indicates a passed of failed test
//based on the type of test provdied and the conditions for the tests specified in the configuration
func getOutcome(testRequest *TestRequest) (string, error) {
	resp := testRequest.Response
	testType := testRequest.TestType
	allowCon := testRequest.AllowCon
	blockCon := testRequest.BlockCon
	//return if test was invalid
	if resp == nil {
		return stringInv, nil
	}
	switch testType {
	//actual = allow, expected == block
	case stringFN:
		//check response code
		if resp.StatusCode != blockCon.Code {
			return stringFN, nil
		}
		//need to check response headers for condition headers if they exist
		if len(blockCon.Headers) > 0 {
			if check := headerCheck(blockCon.Headers, resp); !check {
				return stringFN, nil
			}
		}
		//everything matches and the test passed
		return stringPass, nil

	//actual == block, expected == allow
	case stringFP:
		//check response code.
		if resp.StatusCode == blockCon.Code {
			return stringFP, nil
		}
		//need to check response headers for condition headers if they exist
		if len(allowCon.Headers) > 0 {
			if check := headerCheck(allowCon.Headers, resp); !check {
				return stringFP, nil
			}
		}
		//everything matches and the test passed
		return stringPass, nil
	}
	return "", fmt.Errorf("unknown outcome of test for payload %v from line %v in file %v against location %v", testRequest.Payload, testRequest.Line, testRequest.FileName, testRequest.Location)
}

//defaultRequest sets up a default HTTP request with the default headers
//as specified in the configuration
func defaultRequest(testSet *config.TestSet, method string, data io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, testSet.URI, data)
	if err != nil {
		return nil, err
	}
	//add default headers
	for header, val := range testSet.DefaultHeaders {
		for i := 0; i < len(val); i++ {
			req.Header.Add(header, val[i])
		}
	}
	req.Close = true
	return req, nil
}

//buildRequest places the payload in the correct part of the request depending on the test location
func (a *Application) buildRequest(testRequest *TestRequest, location *config.TestLocation, testSet *config.TestSet) error {
	req, err := defaultRequest(testSet, http.MethodGet, nil)
	if err != nil {
		return err
	}
	req.Close = true
	switch strings.ToLower(location.Location) {
	case "header":
		testRequest.Request = req
		testRequest.Request.Header.Del(location.Key)
		//url encoded header
		if a.TestRun.URLEncodeHeader {
			testRequest.Request.Header.Add(location.Key, url.QueryEscape(testRequest.Payload))
			testRequest.CheckPayload = url.QueryEscape(testRequest.Payload)
			//raw header
		} else {
			testRequest.Request.Header.Add(location.Key, testRequest.Payload)
			testRequest.CheckPayload = testRequest.Payload
		}
		testRequest.Request.ContentLength = int64(0)
		return nil
	case "path":
		testRequest.Request = req
		//url encoded path
		if a.TestRun.URLEncodePath {
			testRequest.Request.URL.Path = "/" + testRequest.Payload
			testRequest.CheckPayload = url.PathEscape(testRequest.Payload)
		} else {
			//raw path
			testRequest.Request.URL = &url.URL{
				Scheme: req.URL.Scheme,
				Host:   req.Host,
				Opaque: "/" + testRequest.Payload,
			}
			testRequest.CheckPayload = testRequest.Payload
		}
		return nil
	case "queryarg":
		testRequest.Request = req
		if a.TestRun.URLEncodeQuery {
			params := url.Values{}
			params.Add(location.Key, testRequest.Payload)
			testRequest.Request.URL.RawQuery = params.Encode()
			testRequest.CheckPayload = params.Encode()
		} else {
			testRequest.Request.URL.RawQuery = fmt.Sprintf("%s=%s", location.Key, testRequest.Payload)
			testRequest.CheckPayload = testRequest.Payload
		}
		return nil
	case "body":
		//url encoded body
		if a.TestRun.PostBodyType == "urlencoded" {
			data := &url.Values{}
			data.Add(location.Key, testRequest.Payload)
			postReq, err := defaultRequest(testSet, http.MethodPost, strings.NewReader(data.Encode()))
			if err != nil {
				return err
			}
			testRequest.Request = postReq
			testRequest.Request.Close = true
			testRequest.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			testRequest.CheckPayload = url.QueryEscape(testRequest.Payload)
		} else if a.TestRun.PostBodyType == "json" {
			//json body
			jsonStr := []byte(`{"` + location.Key + `":"` + testRequest.Payload + `"}`)
			postReq, err := defaultRequest(testSet, http.MethodPost, bytes.NewBuffer(jsonStr))
			if err != nil {
				return err
			}
			testRequest.Request = postReq
			testRequest.Request.Close = true
			testRequest.Request.Header.Set("Content-Type", "application/json")
			testRequest.CheckPayload = testRequest.Payload
		} else {
			//raw body
			postStr := []byte(location.Key + "=" + testRequest.Payload)
			postReq, err := defaultRequest(testSet, http.MethodPost, bytes.NewBuffer(postStr))
			if err != nil {
				return err
			}
			testRequest.Request = postReq
			testRequest.Request.Close = true
			testRequest.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			testRequest.CheckPayload = testRequest.Payload
		}
		return nil
	case "cookie":
		testRequest.Request = req
		//encoded cookies are base64 encoded per the recommendation of RFC 6265 section 4.1.1
		if a.TestRun.B64EncodeCookie {
			testRequest.Request.Header.Add("Cookie", fmt.Sprintf(`%v=%v`, location.Key, base64.RawStdEncoding.EncodeToString([]byte(testRequest.Payload))))
			testRequest.CheckPayload = base64.RawStdEncoding.EncodeToString([]byte(testRequest.Payload))
		} else {
			//force add the payload to the cookie. This bypasses request.AddCookie() which will strip invalid
			//characters. We don't want to strip the characters, we want to show an invalid test.
			testRequest.Request.Header.Add("Cookie", fmt.Sprintf(`%v=%v`, location.Key, testRequest.Payload))
			testRequest.CheckPayload = testRequest.Payload
		}
		return nil
	default:
		return fmt.Errorf("Unknown location: %v", location.Location)
	}
}

//headerCheck looks to see if the response contains the headers and values designated
//in the headers map. It returns true if all headers are present and false otherwise.
func headerCheck(headers []*config.Header, resp *http.Response) bool {
	for _, header := range headers {
		//if the key does not exist in the response, return false
		respVals, ok := resp.Header[header.Header]
		if !ok {
			return false
		}
		// look in slice of values for value defined in the block conditon
		// if it does not exist, return false
		if containOK := stringContains(respVals, header.Value); !containOK {
			return false
		}
	}
	return true
}

//stringContains returns true if slice s contians string b
func stringContains(s []string, b string) bool {
	for _, a := range s {
		if a == b {
			return true
		}
	}
	return false
}

//intContains returns true if slice s contians int b
func intContains(s []int, b int) bool {
	for _, a := range s {
		if a == b {
			return true
		}
	}
	return false
}
