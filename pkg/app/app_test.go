package app

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/signalsciences/waf-testing-framework/pkg/config"
	"github.com/signalsciences/waf-testing-framework/pkg/results"
	"github.com/sirupsen/logrus"
)

func initResponse(resp *http.Response) {

	resp.Status = "200 OK"
	resp.StatusCode = 200
	resp.Proto = "HTTP/1.1"
	resp.ProtoMajor = 1
	resp.ProtoMinor = 1
	resp.Header = map[string][]string{
		"Content-Encoding": {"gzip"},
		"Connection":       {"close"},
		"Content-Type":     {"text/html"},
		"Cache-Control":    {"no-store", "no-cache", "must-revalidate", "post-check=0", "pre-check=0"},
		"Date":             {"Wed, 08 Apr 2020 14:15:02 GMT"},
		"Expires":          {"Thu, 19 Nov 1981 08:52:00 GMT"},
		"Pragma":           {"no-cache"},
		"Server":           {"Apache/2.4.7 (Ubuntu)"},
		"Set-Cookie":       {"PHPSESSID=85g87vnie51ceh2h8t0dkhmis3; path=/"},
		"Vary":             {"Accept-Encoding"},
		"X-Powered-By":     {"PHP/5.5.9-1ubuntu4.29"},
	}
	resp.Body = http.NoBody
	resp.ContentLength = 0
}

var header1 = &config.Header{
	Header: "Foo",
	Value:  "Bar",
}
var header2 = &config.Header{
	Header: "Lorem",
	Value:  "Ipsum",
}
var header3 = &config.Header{
	Header: "Hello",
	Value:  "World",
}
var header4 = &config.Header{
	Header: "Foo",
	Value:  "World",
}

var testRun = &config.TestRun{
	Locations: []*config.TestLocation{{Location: "header", Key: "foo"}},
	TestFiles: []*config.TestFile{{File: filepath.FromSlash("../testdata/payloads/false_positives/fp.txt"), TestType: "falsePositive"}},
	TestSets: []*config.TestSet{
		{
			Name:           "Test1",
			AllowCondition: &config.Condition{Code: 200, Headers: nil},
			BlockCondition: &config.Condition{Code: 406, Headers: nil},
		},
	},
}

type MockClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	return GetDoFunc(req)
}

var (
	GetDoFunc func(req *http.Request) (*http.Response, error)
)

func TestRequestWoker(t *testing.T) {
	testsChan := make(chan *TestRequest, 1)
	doneQueuingChan := make(chan struct{}, 1)
	stopChan := make(chan struct{}, 1)
	resultsChan := make(chan *results.TestResult, 1)
	rateLimiter := time.NewTicker(time.Second / time.Duration(5))
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	mockClient := &MockClient{}
	app := &Application{
		TestRun:         testRun,
		Log:             log,
		TestsChan:       testsChan,
		DoneQueuingChan: doneQueuingChan,
		ResultsChan:     resultsChan,
		Client:          mockClient,
		Results:         results.InitResults(testRun),
		RateLimiter:     rateLimiter,
	}
	testRequest := &TestRequest{
		SetName:  "Test1",
		Location: "header",
		FileName: "fp.txt",
		TestType: "falsePositive",
		Line:     1,
		Payload:  "LOCK AND KEY",
		AllowCon: &config.Condition{Code: 200, Headers: nil},
		BlockCon: &config.Condition{Code: 406, Headers: nil},
	}
	request, _ := http.NewRequest("GET", "http://localhost", nil)
	testRequest.Request = request
	wantRequest, _ := httputil.DumpRequestOut(testRequest.Request, false)
	wantResult := &results.TestResult{
		SetName:  "Test1",
		Location: "header",
		FileName: "fp.txt",
		Line:     1,
		Payload:  "LOCK AND KEY",
		Outcome:  "falsePositive",
		Request:  string(wantRequest),
		Response: "HTTP/0.0 406 Not Acceptable\r\nContent-Length: 0\r\n\r\n",
	}
	//mock client for the request
	GetDoFunc = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    406,
			Status:        "Not Acceptable",
			ContentLength: 0,
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("body"))),
		}, nil
	}
	//create a worker
	go func() {
		app.RequestWG.Add(1)
		app.requestWorker(1, stopChan)
	}()

	//add the data to the queue
	app.TestsChan <- testRequest

	//stop the worker
	close(app.DoneQueuingChan)
	app.RequestWG.Wait()

	t.Run("result check", func(t *testing.T) {
		out := <-app.ResultsChan
		if ok := cmp.Equal(out, wantResult, cmpopts.IgnoreUnexported(http.Request{})); !ok {
			diff := cmp.Diff(wantResult, out, cmpopts.IgnoreUnexported(http.Request{}))
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
	close(app.ResultsChan)
	close(stopChan)
}

func TestResultWoker(t *testing.T) {
	doneProcessingChan := make(chan struct{}, 1)
	stopChan := make(chan struct{}, 1)
	resultsChan := make(chan *results.TestResult, 1)
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	app := &Application{
		TestRun:            testRun,
		Log:                log,
		DoneProcessingChan: doneProcessingChan,
		ResultsChan:        resultsChan,
		Results:            results.InitResults(testRun),
	}
	testResult := &results.TestResult{
		SetName:  "Test1",
		Location: "header",
		FileName: filepath.FromSlash("false_positives/fp.txt"),
		Line:     1,
		Payload:  "LOCK AND KEY",
		Outcome:  "falsePositive",
		Request:  "request",
		Response: "response",
	}

	wantResult := &results.Results{
		Config: testRun,
		SetCounts: map[string]*results.SetCounts{
			"Test1": {
				FpCount:     1,
				PassedCount: 0,
				TotalCount:  0,
			},
		},
		FileResults: map[string]*results.FileResult{
			filepath.FromSlash("false_positives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*results.PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*results.SetResult{
							"Test1": {
								Locations: map[string]*results.TestResult{
									"header": {
										SetName:  "Test1",
										FileName: filepath.FromSlash("false_positives/fp.txt"),
										Line:     1,
										Payload:  "LOCK AND KEY",
										Location: "header",
										Outcome:  "falsePositive",
										Request:  "request",
										Response: "response",
									},
								},
							},
						},
					},
				},
			},
		},
		Reporting: &results.Reporting{
			OutputDir: filepath.FromSlash("./output"),
		},
	}
	//create a worker
	go func() {
		app.ResultWG.Add(1)
		app.resultWorker(1, stopChan)
	}()

	//add the data to the queue
	app.ResultsChan <- testResult
	time.Sleep(1 * time.Second)
	//stop the worker
	close(app.DoneProcessingChan)
	app.ResultWG.Wait()

	t.Run("result check", func(t *testing.T) {
		out := app.Results
		if ok := cmp.Equal(out, wantResult, cmpopts.IgnoreUnexported(http.Request{}), cmpopts.IgnoreFields(results.Results{}, "StartTime", "EndTime")); !ok {
			diff := cmp.Diff(wantResult, out, cmpopts.IgnoreUnexported(http.Request{}), cmpopts.IgnoreFields(results.Results{}, "StartTime", "EndTime"))
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
	close(stopChan)
	close(app.ResultsChan)
}

func TestQueueTests(t *testing.T) {
	testsChan := make(chan *TestRequest, 1)
	doneQueuingChan := make(chan struct{}, 1)
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	app := &Application{
		TestRun:         testRun,
		Log:             log,
		TestsChan:       testsChan,
		DoneQueuingChan: doneQueuingChan,
	}
	wantRequest := &TestRequest{
		SetName:      "Test1",
		Location:     "header",
		FileName:     filepath.FromSlash("false_positives/fp.txt"),
		TestType:     "falsePositive",
		Line:         1,
		Payload:      "LOCK AND KEY",
		CheckPayload: "LOCK AND KEY",
		AllowCon:     &config.Condition{Code: 200, Headers: nil},
		BlockCon:     &config.Condition{Code: 406, Headers: nil},
		Request: &http.Request{
			Method:     "GET",
			URL:        &url.URL{Host: ""},
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     http.Header{"Foo": {"LOCK AND KEY"}},
			Close:      true,
		},
	}
	tests := []struct {
		name string
		app  *Application
		want *TestRequest
	}{
		{
			name: "queuetests",
			app:  app,
			want: wantRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.app.queueTests()
			out := <-tt.app.TestsChan
			close(tt.app.TestsChan)
			if ok := cmp.Equal(out, tt.want, cmpopts.IgnoreUnexported(http.Request{})); !ok {
				diff := cmp.Diff(tt.want, out, cmpopts.IgnoreUnexported(http.Request{}))
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetOutcome(t *testing.T) {

	BlockConditionHeaders := &config.Condition{
		Code:    406,
		Headers: []*config.Header{header1, header2},
	}
	AllowConditionHeaders := &config.Condition{
		Code:    200,
		Headers: []*config.Header{header1, header2},
	}

	BlockConditionNoHeaders := &config.Condition{
		Code:    406,
		Headers: nil,
	}

	BlockConditionInvalidCode := &config.Condition{
		Code:    403,
		Headers: nil,
	}

	FNCodeMismatch, FNHeaderMismatch, FNValid, FPValid, FPCodeMismatch, FPHeaderMismatch := new(http.Response), new(http.Response), new(http.Response), new(http.Response), new(http.Response), new(http.Response)
	//FN code mismatch. want 406, get 200
	initResponse(FNCodeMismatch)
	//FN header mismatch. code match, but header missing
	initResponse(FNHeaderMismatch)
	FNHeaderMismatch.Status = "406 Not Allowed"
	FNHeaderMismatch.StatusCode = 406
	//FN valid no FN. code and headers match
	initResponse(FNValid)
	FNValid.Status = "406 Not Allowed"
	FNValid.StatusCode = 406
	FNValid.Header.Add("Foo", "Bar")
	FNValid.Header.Add("Lorem", "Ipsum")
	//FP code mismatch. want 200, get 406
	initResponse(FPCodeMismatch)
	FPCodeMismatch.Status = "406 Not Allowed"
	FPCodeMismatch.StatusCode = 406
	//FP header mismatch. code match, but header missing
	initResponse(FPHeaderMismatch)
	//FP valid no FP. code and headers match
	initResponse(FPValid)
	FPValid.Header.Add("Foo", "Bar")
	FPValid.Header.Add("Lorem", "Ipsum")

	tests := []struct {
		name        string
		testRequest *TestRequest
		want        string
		wantErr     bool
	}{
		{
			name: "Invalid",
			testRequest: &TestRequest{
				Response: nil,
				TestType: stringFN,
				BlockCon: BlockConditionInvalidCode,
			},
			want:    stringInv,
			wantErr: false,
		},
		{
			name: "FNCodeMismatch",
			testRequest: &TestRequest{
				Response: FNCodeMismatch,
				TestType: stringFN,
				BlockCon: BlockConditionInvalidCode,
			},
			want:    stringFN,
			wantErr: false,
		},
		{
			name: "FNHeaderMismatch",
			testRequest: &TestRequest{
				Response: FNHeaderMismatch,
				TestType: stringFN,
				BlockCon: BlockConditionHeaders,
			},
			want:    stringFN,
			wantErr: false,
		},
		{
			name: "FNValid",
			testRequest: &TestRequest{
				Response: FNValid,
				TestType: stringFN,
				BlockCon: BlockConditionNoHeaders,
			},
			want:    stringPass,
			wantErr: false,
		},
		{
			name: "FPCodeMismatch",
			testRequest: &TestRequest{
				Response: FPCodeMismatch,
				TestType: stringFP,
				BlockCon: BlockConditionNoHeaders,
			},
			want:    stringFP,
			wantErr: false,
		},
		{
			name: "FPHeaderMismatch",
			testRequest: &TestRequest{
				Response: FPHeaderMismatch,
				TestType: stringFP,
				AllowCon: AllowConditionHeaders,
				BlockCon: BlockConditionNoHeaders,
			},
			want:    stringFP,
			wantErr: false,
		},
		{
			name: "FPValidNoFP",
			testRequest: &TestRequest{
				Response: FPValid,
				TestType: stringFP,
				BlockCon: BlockConditionNoHeaders,
				AllowCon: AllowConditionHeaders,
			},
			want:    stringPass,
			wantErr: false,
		},
		{
			name: "Error",
			testRequest: &TestRequest{
				Response: FPValid,
				TestType: "unknown",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := getOutcome(tt.testRequest)
			if err != nil && !tt.wantErr {
				t.Error(err)
			}
			if err != nil && tt.wantErr {
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if err == nil && !tt.wantErr {
				if ok := cmp.Equal(tt.want, got); !ok {
					diff := cmp.Diff(tt.want, got)
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}

		})
	}
}

func TestDefaultRequest(t *testing.T) {
	httpGetWant, _ := http.NewRequest("GET", "http://testURI", nil)
	httpGetWant.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpGetWant.Header.Add("Accept-Encoding", "gzip, deflate")
	httpGetWant.Close = true
	dataVal := &url.Values{
		"Foo": {"Bar"},
	}
	httpPostWant, _ := http.NewRequest("POST", "http://testURI", strings.NewReader(dataVal.Encode()))
	httpPostWant.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpPostWant.Header.Add("Accept-Encoding", "gzip, deflate")
	httpPostWant.Close = true
	tests := []struct {
		name    string
		testSet *config.TestSet
		method  string
		data    io.Reader
		want    *http.Request
		wantErr bool
	}{
		{
			name: "noBody",
			testSet: &config.TestSet{
				URI: "http://testURI",
				DefaultHeaders: map[string][]string{
					"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
					"Accept-Encoding": {"gzip, deflate"},
				},
			},
			method:  http.MethodGet,
			data:    nil,
			want:    httpGetWant,
			wantErr: false,
		},
		{
			name: "Body",
			testSet: &config.TestSet{
				URI: "http://testURI",
				DefaultHeaders: map[string][]string{
					"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
					"Accept-Encoding": {"gzip, deflate"},
				},
			},
			method:  http.MethodPost,
			data:    strings.NewReader(dataVal.Encode()),
			want:    httpPostWant,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultRequest(tt.testSet, tt.method, tt.data)
			if err != nil && !tt.wantErr {
				t.Error(err)
			}
			if err != nil && tt.wantErr {
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if err == nil && !tt.wantErr {
				if ok := cmp.Equal(tt.want, got, cmpopts.IgnoreUnexported(http.Request{}, strings.Reader{}), cmpopts.IgnoreFields(http.Request{}, "GetBody")); !ok {
					diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(http.Request{}, strings.Reader{}), cmpopts.IgnoreFields(http.Request{}, "GetBody"))
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestBuildRequest(t *testing.T) {
	a := Application{
		TestRun: &config.TestRun{
			URLEncodePath:   false,
			URLEncodeHeader: false,
			URLEncodeQuery:  false,
			B64EncodeCookie: false,
			PostBodyType:    "urlencoded",
			TestSets: []*config.TestSet{
				{
					DefaultHeaders: map[string][]string{
						"Lorem": {"Ipsum"},
					},
					URI: "http://testhost",
				},
			},
		},
	}
	testRequest := &TestRequest{
		Payload: "bar!",
	}
	headerWantRaw := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Foo": {"bar!"}, "Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	headerWantEncoded := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Foo": {"bar%21"}, "Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	pathWantRaw := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
			Opaque: "/bar!",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	pathWantEncoded := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
			Opaque: "/bar%21",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	queryargWantRaw := &http.Request{
		URL: &url.URL{
			Host:     "testhost",
			Scheme:   "http",
			RawQuery: "Foo=bar!",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	queryargWantEncoded := &http.Request{
		URL: &url.URL{
			Host:     "testhost",
			Scheme:   "http",
			RawQuery: "Foo=bar%21",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	bodyURLWant := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:        "POST",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Lorem": {"Ipsum"}},
		ContentLength: 7,
		Host:          "testhost",
		Body:          ioutil.NopCloser(strings.NewReader("Foo=bar%21")),
		Close:         true,
	}
	bodyJSONWant := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:        "POST",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}, "Lorem": {"Ipsum"}},
		ContentLength: 13,
		Host:          "testhost",
		Body:          ioutil.NopCloser(strings.NewReader(`{"Foo":"bar!"}`)),
		Close:         true,
	}

	bodyRawWant := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:        "POST",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Lorem": {"Ipsum"}},
		ContentLength: 7,
		Host:          "testhost",
		Body:          ioutil.NopCloser(strings.NewReader("Foo=bar!")),
		Close:         true,
	}
	cookieWantRaw := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Cookie": {"Foo=bar!"}, "Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	cookieWantEncoded := &http.Request{
		URL: &url.URL{
			Host:   "testhost",
			Scheme: "http",
		},
		Method:     "GET",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Cookie": {"Foo=YmFyIQ"}, "Lorem": {"Ipsum"}},
		Host:       "testhost",
		Close:      true,
	}
	tests := []struct {
		name        string
		testRequest *TestRequest
		location    *config.TestLocation
		testSet     *config.TestSet
		encoded     bool
		postType    string
		want        *http.Request
		wantErr     bool
	}{
		{
			name:        "headerRequestRaw",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     false,
			postType:    "",
			location: &config.TestLocation{
				Location: "header",
				Key:      "Foo",
			},
			want:    headerWantRaw,
			wantErr: false,
		},
		{
			name:        "headerRequestEncoded",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     true,
			postType:    "",
			location: &config.TestLocation{
				Location: "header",
				Key:      "Foo",
			},
			want:    headerWantEncoded,
			wantErr: false,
		},
		{
			name:        "pathRequestRaw",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     false,
			postType:    "",
			location: &config.TestLocation{
				Location: "path",
			},
			want:    pathWantRaw,
			wantErr: false,
		},
		{
			name:        "pathRequestEncoded",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     true,
			postType:    "",
			location: &config.TestLocation{
				Location: "path",
			},
			want:    pathWantEncoded,
			wantErr: false,
		},
		{
			name:        "queryargRequestRaw",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     false,
			postType:    "",
			location: &config.TestLocation{
				Location: "queryarg",
				Key:      "Foo",
			},
			want:    queryargWantRaw,
			wantErr: false,
		},
		{
			name:        "queryargRequestEncoded",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     true,
			postType:    "",
			location: &config.TestLocation{
				Location: "queryarg",
				Key:      "Foo",
			},
			want:    queryargWantEncoded,
			wantErr: false,
		},
		{
			name:        "bodyURLEncodeRequest",
			testRequest: testRequest,
			encoded:     true,
			postType:    "urlencoded",
			testSet:     a.TestRun.TestSets[0],
			location: &config.TestLocation{
				Location: "body",
				Key:      "Foo",
			},
			want:    bodyURLWant,
			wantErr: false,
		},
		{
			name:        "bodyJSONRequest",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     true,
			postType:    "json",
			location: &config.TestLocation{
				Location: "body",
				Key:      "Foo",
			},
			want:    bodyJSONWant,
			wantErr: false,
		},
		{
			name:        "bodyRawRequest",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     false,
			postType:    "raw",
			location: &config.TestLocation{
				Location: "body",
				Key:      "Foo",
			},
			want:    bodyRawWant,
			wantErr: false,
		},
		{
			name:        "cookieRequestRaw",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     false,
			postType:    "",
			location: &config.TestLocation{
				Location: "cookie",
				Key:      "Foo",
			},
			want:    cookieWantRaw,
			wantErr: false,
		},
		{
			name:        "cookieRequestEncoded",
			testRequest: testRequest,
			testSet:     a.TestRun.TestSets[0],
			encoded:     true,
			postType:    "",
			location: &config.TestLocation{
				Location: "cookie",
				Key:      "Foo",
			},
			want:    cookieWantEncoded,
			wantErr: false,
		},
		{
			name:        "invalidLocation",
			testRequest: testRequest,
			encoded:     false,
			postType:    "",
			testSet:     a.TestRun.TestSets[0],
			location: &config.TestLocation{
				Location: "nowhere",
				Key:      "Foo",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.encoded {
				a.TestRun.B64EncodeCookie = true
				a.TestRun.URLEncodeHeader = true
				a.TestRun.URLEncodePath = true
				a.TestRun.URLEncodeQuery = true
			} else {
				a.TestRun.B64EncodeCookie = false
				a.TestRun.URLEncodeHeader = false
				a.TestRun.URLEncodePath = false
				a.TestRun.URLEncodeQuery = false
			}
			a.TestRun.PostBodyType = tt.postType
			err := a.buildRequest(tt.testRequest, tt.location, tt.testSet)
			if err != nil && !tt.wantErr {
				t.Error(err)
			}
			if err != nil && tt.wantErr {
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if err == nil && !tt.wantErr {
				wantBytes, _ := httputil.DumpRequest(tt.want, true)
				gotBytes, _ := httputil.DumpRequest(tt.testRequest.Request, true)
				if ok := cmp.Equal(wantBytes, gotBytes); !ok {
					diff := cmp.Diff(wantBytes, gotBytes)
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestHeaderCheck(t *testing.T) {
	baseResp, testResp1, testResp2 := new(http.Response), new(http.Response), new(http.Response)
	initResponse(baseResp)
	initResponse(testResp1)
	testResp1.Header.Add("Foo", "Bar")
	initResponse(testResp2)
	testResp2.Header.Add("Foo", "Bar")
	testResp2.Header.Add("Lorem", "Ipsum")
	tests := []struct {
		name    string
		headers []*config.Header
		resp    *http.Response
		want    bool
	}{
		{
			name:    "allHeaders",
			headers: []*config.Header{header1, header2},
			resp:    testResp2,
			want:    true,
		},
		{
			name:    "onlyOneHeader",
			headers: []*config.Header{header1, header2},
			resp:    testResp1,
			want:    false,
		},
		{
			name:    "noHeader",
			headers: []*config.Header{header3},
			resp:    baseResp,
			want:    false,
		},
		{
			name:    "wrongValue",
			headers: []*config.Header{header4},
			resp:    testResp1,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerCheck(tt.headers, tt.resp)
			if got != tt.want {
				diff := cmp.Diff(tt.headers, tt.resp.Header)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStringContains(t *testing.T) {
	tests := []struct {
		name      string
		source    []string
		substring string
		want      bool
	}{
		{
			name:      "contains",
			source:    []string{"Hello World", " Lorem Ipsum"},
			substring: "Hello World",
			want:      true,
		},
		{
			name:      "doesNotContain",
			source:    []string{"Hello World", " Lorem Ipsum"},
			substring: "Foo",
			want:      false,
		},
		{
			name:      "caseSensitivity",
			source:    []string{"Hello World", " Lorem Ipsum"},
			substring: "hello World",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringContains(tt.source, tt.substring)
			if got != tt.want {
				t.Errorf("want: %v\n got: %v", tt.want, got)
			}
		})
	}
}

func TestIntContains(t *testing.T) {
	tests := []struct {
		name   string
		source []int
		subInt int
		want   bool
	}{
		{
			name:   "contains",
			source: []int{1, 2, 3, 4},
			subInt: 3,
			want:   true,
		},
		{
			name:   "doesNotContain",
			source: []int{1, 2, 3, 4},
			subInt: 5,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intContains(tt.source, tt.subInt)
			if got != tt.want {
				t.Errorf("want: %v\n got: %v", tt.want, got)
			}
		})
	}
}
