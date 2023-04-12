package results

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/signalsciences/waf-testing-framework/pkg/config"
)

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

var testRun2 = &config.TestRun{
	Locations: []*config.TestLocation{
		{
			Location: "header",
			Key:      "foo",
		},
		{
			Location: "body",
			Key:      "bar",
		},
	},
	TestFiles: []*config.TestFile{{File: filepath.FromSlash("../testdata/payloads/false_positives/fp.txt"), TestType: "falsePositive"}},
	TestSets: []*config.TestSet{
		{
			Name:           "Test1",
			AllowCondition: &config.Condition{Code: 200, Headers: nil},
			BlockCondition: &config.Condition{Code: 406, Headers: nil},
		},
		{
			Name:           "Test2",
			AllowCondition: &config.Condition{Code: 201, Headers: nil},
			BlockCondition: &config.Condition{Code: 403, Headers: nil},
		},
		{
			Name:           "Test3",
			AllowCondition: &config.Condition{Code: 202, Headers: nil},
			BlockCondition: &config.Condition{Code: 404, Headers: nil},
		},
	},
}

var result = &Results{
	Config: testRun2,
	SetCounts: map[string]*SetCounts{
		"Test1": {
			FpCount:     1,
			FpPercent:   100,
			PassedCount: 0,
			TotalCount:  2,
			InvCount:    1,
			FailPercent: 50,
		},
		"Test2": {
			PassedCount: 2,
			TotalCount:  2,
		},
		"Test3": {
			PassedCount: 1,
			FpCount:     1,
			FpPercent:   50,
			FailPercent: 50,
			TotalCount:  2,
		},
	},
	FileResults: map[string]*FileResult{
		filepath.FromSlash("false_positives/fp.txt"): {
			FailedLines: []int{1},
			PayloadResults: map[int]*PayloadResult{
				1: {
					Line:    1,
					Payload: "LOCK AND KEY",
					SetResults: map[string]*SetResult{
						"Test1": {
							Locations: map[string]*TestResult{
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
								"body": {
									SetName:  "Test1",
									FileName: filepath.FromSlash("false_positives/fp.txt"),
									Line:     1,
									Payload:  "LOCK AND KEY",
									Location: "body",
									Outcome:  "invalid",
									Request:  "request",
									Response: "response",
								},
							},
						},
						"Test3": {
							Locations: map[string]*TestResult{
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
	Reporting: &Reporting{
		OutputDir: filepath.FromSlash("../testdata"),
	},
}
var wantReport = &OverallReport{
	TestSets:  []string{"Test1", "Test2", "Test3"},
	Locations: []string{"body", "header"},
	Matrix: []*FileReport{
		{
			FileName: filepath.FromSlash("false_positives/fp.txt"),
			RowReport: []*RowReport{
				{
					Line:    1,
					Payload: "LOCK AND KEY",
					SetReport: map[string][]int{
						"Test1": {2, 1},
						"Test2": {0, 0},
						"Test3": {0, 1},
					},
				},
			},
		},
	},
	Config: "{\n  \"PayloadDir\": \"\",\n  \"URLEncodePath\": false,\n  \"URLEncodeQuery\": false,\n  \"URLEncodeHeader\": false,\n  \"B64EncodeCookie\": false,\n  \"PostBodyType\": \"\",\n  \"Locations\": [\n    {\n      \"Location\": \"header\",\n      \"Key\": \"foo\"\n    },\n    {\n      \"Location\": \"body\",\n      \"Key\": \"bar\"\n    }\n  ],\n  \"TestSets\": [\n    {\n      \"Name\": \"Test1\",\n      \"URI\": \"\",\n      \"DefaultHeaders\": null,\n      \"AllowCondition\": {\n        \"Code\": 200,\n        \"Headers\": null\n      },\n      \"BlockCondition\": {\n        \"Code\": 406,\n        \"Headers\": null\n      }\n    },\n    {\n      \"Name\": \"Test2\",\n      \"URI\": \"\",\n      \"DefaultHeaders\": null,\n      \"AllowCondition\": {\n        \"Code\": 201,\n        \"Headers\": null\n      },\n      \"BlockCondition\": {\n        \"Code\": 403,\n        \"Headers\": null\n      }\n    },\n    {\n      \"Name\": \"Test3\",\n      \"URI\": \"\",\n      \"DefaultHeaders\": null,\n      \"AllowCondition\": {\n        \"Code\": 202,\n        \"Headers\": null\n      },\n      \"BlockCondition\": {\n        \"Code\": 404,\n        \"Headers\": null\n      }\n    }\n  ]\n}",
	Results: &Results{
		Config: &config.TestRun{
			Locations: []*config.TestLocation{
				{
					Location: "header",
					Key:      "foo",
				},
				{
					Location: "body",
					Key:      "bar",
				},
			},
			TestFiles: []*config.TestFile{
				{
					File:     filepath.FromSlash("../testdata/payloads/false_positives/fp.txt"),
					TestType: "falsePositive",
				},
			},
			TestSets: []*config.TestSet{
				{
					Name:           "Test1",
					AllowCondition: &config.Condition{Code: 200},
					BlockCondition: &config.Condition{Code: 406},
				},
				{
					Name:           "Test2",
					AllowCondition: &config.Condition{Code: 201},
					BlockCondition: &config.Condition{Code: 403},
				},
				{
					Name:           "Test3",
					AllowCondition: &config.Condition{Code: 202},
					BlockCondition: &config.Condition{Code: 404},
				},
			},
		},
		SetCounts: map[string]*SetCounts{
			"Test1": {
				FpCount:     1,
				FpPercent:   100,
				InvCount:    1,
				FailPercent: 50,
				TotalCount:  2,
			},
			"Test2": {
				PassedCount: 2,
				TotalCount:  2,
			},
			"Test3": {
				FpCount:     1,
				FpPercent:   50,
				PassedCount: 1,
				FailPercent: 50,
				TotalCount:  2,
			},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_positives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*SetResult{
							"Test1": {
								Locations: map[string]*TestResult{
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
									"body": {
										SetName:  "Test1",
										FileName: filepath.FromSlash("false_positives/fp.txt"),
										Line:     1,
										Payload:  "LOCK AND KEY",
										Location: "body",
										Outcome:  "invalid",
										Request:  "request",
										Response: "response",
									},
								},
							},
							"Test3": {
								Locations: map[string]*TestResult{
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
		Reporting: &Reporting{OutputDir: filepath.FromSlash("../testdata")},
	},
}

func TestInitResults(t *testing.T) {
	initResult := &Results{
		Config: testRun,
		SetCounts: map[string]*SetCounts{
			"Test1": {},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_positives/fp.txt"): {
				PayloadResults: map[int]*PayloadResult{},
			},
		},
		Reporting: &Reporting{OutputDir: filepath.FromSlash("./output")},
	}

	tests := []struct {
		name    string
		testRun *config.TestRun
		want    *Results
	}{
		{
			name:    "initialization",
			testRun: testRun,
			want:    initResult,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InitResults(tt.testRun)
			if ok := cmp.Equal(got, tt.want, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime")); !ok {
				diff := cmp.Diff(tt.want, got, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime"))
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessResults(t *testing.T) {
	rawFPResult := &Results{
		Config: testRun,
		SetCounts: map[string]*SetCounts{
			"Test1": {
				FpCount:          1,
				TotalFPTestCount: 1,
				PassedCount:      0,
				TotalCount:       1,
			},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_positives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*SetResult{
							"Test1": {
								Locations: map[string]*TestResult{
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
	}
	wantFPResult := &Results{
		Config: testRun,
		SetCounts: map[string]*SetCounts{
			"Test1": {
				FpCount:          1,
				TotalFPTestCount: 1,
				FpPercent:        100,
				FnCount:          0,
				TotalFNTestCount: 0,
				FnPercent:        0.00,
				PassedCount:      0,
				TotalCount:       1,
				FailPercent:      100,
			},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_positives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*SetResult{
							"Test1": {
								Locations: map[string]*TestResult{
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
	}
	rawFNResult := &Results{
		Config: testRun,
		SetCounts: map[string]*SetCounts{
			"Test1": {
				FnCount:          1,
				TotalFNTestCount: 1,
				PassedCount:      0,
				TotalCount:       1,
			},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_negatives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*SetResult{
							"Test1": {
								Locations: map[string]*TestResult{
									"header": {
										SetName:  "Test1",
										FileName: filepath.FromSlash("false_negatives/fp.txt"),
										Line:     1,
										Payload:  "LOCK AND KEY",
										Location: "header",
										Outcome:  "falseNegative",
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
	}
	wantFNResult := &Results{
		Config: testRun,
		SetCounts: map[string]*SetCounts{
			"Test1": {
				FnCount:          1,
				TotalFNTestCount: 1,
				FnPercent:        100,
				FpCount:          0,
				TotalFPTestCount: 0,
				FpPercent:        0.00,
				PassedCount:      0,
				TotalCount:       1,
				FailPercent:      100,
			},
		},
		FileResults: map[string]*FileResult{
			filepath.FromSlash("false_negatives/fp.txt"): {
				FailedLines: []int{1},
				PayloadResults: map[int]*PayloadResult{
					1: {
						Line:    1,
						Payload: "LOCK AND KEY",
						SetResults: map[string]*SetResult{
							"Test1": {
								Locations: map[string]*TestResult{
									"header": {
										SetName:  "Test1",
										FileName: filepath.FromSlash("false_negatives/fp.txt"),
										Line:     1,
										Payload:  "LOCK AND KEY",
										Location: "header",
										Outcome:  "falseNegative",
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
	}
	tests := []struct {
		name   string
		result *Results
		want   *Results
	}{
		{
			name:   "FPprocessing",
			result: rawFPResult,
			want:   wantFPResult,
		},
		{
			name:   "FNprocessing",
			result: rawFNResult,
			want:   wantFNResult,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.result.ProcessResults()
			if ok := cmp.Equal(tt.result, tt.want, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime")); !ok {
				diff := cmp.Diff(tt.want, tt.result, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime"))
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReportData(t *testing.T) {

	tests := []struct {
		name   string
		result *Results
		want   *OverallReport
	}{
		{
			name:   "reportTranslation",
			result: result,
			want:   wantReport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reportOut := tt.result.ReportData()
			if ok := cmp.Equal(reportOut, tt.want, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime")); !ok {
				diff := cmp.Diff(tt.want, reportOut, cmpopts.IgnoreFields(Results{}, "StartTime", "EndTime"))
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateReports(t *testing.T) {
	var expectedSumReport, expectedDetailsReport, expectedJSON string
	//handle windows path separator in expected reports
	if runtime.GOOS == "windows" {
		expectedSumReport = filepath.FromSlash("../testdata/expectedWinSumReport.html")
		expectedDetailsReport = filepath.FromSlash("../testdata/expectedWinDetailReport.html")
		expectedJSON = filepath.FromSlash("../testdata/expectedWinJSON.json")
	} else {
		expectedSumReport = filepath.FromSlash("../testdata/expectedSumReport.html")
		expectedDetailsReport = filepath.FromSlash("../testdata/expectedDetailReport.html")
		expectedJSON = filepath.FromSlash("../testdata/expectedJSON.json")
	}
	wantSumReport, err := ioutil.ReadFile(expectedSumReport)
	wantDetailsReport, err := ioutil.ReadFile(expectedDetailsReport)
	wantJSON, err := ioutil.ReadFile(expectedJSON)
	if err != nil {
		t.Errorf("unable to open expected report: %v\n", err)
	}
	tests := []struct {
		name        string
		results     *Results
		wantSum     string
		wantDetails string
		wantJSON    string
		wantErr     bool
	}{
		{
			name:        "report",
			results:     result,
			wantSum:     string(wantSumReport),
			wantDetails: string(wantDetailsReport),
			wantJSON:    string(wantJSON),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outDir := tt.results.Reporting.OutputDir
			err = tt.results.GenerateReports()
			if err != nil && !tt.wantErr {
				t.Errorf("unable to create report: %v\n", err)
			}
			if err != nil && tt.wantErr {
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if err == nil && !tt.wantErr {
				gotSumReport, err := ioutil.ReadFile(outDir + filepath.FromSlash("/summary.html"))
				if err != nil {
					t.Errorf("unable to open created report: %v\n", err)
				}
				cleanWantSum := spaceStringsBuilder(tt.wantSum)
				cleanGotSum := spaceStringsBuilder(string(gotSumReport))
				if string(cleanGotSum) != cleanWantSum {
					diff := cmp.Diff(cleanWantSum, cleanGotSum)
					t.Errorf("summary mismatch (-want +got):\n%s", diff)
				}
				gotDetailsReport, err := ioutil.ReadFile(outDir + filepath.FromSlash("/details.html"))
				if err != nil {
					t.Errorf("unable to open created report: %v\n", err)
				}
				cleanWantDetails := spaceStringsBuilder(tt.wantDetails)
				cleanGotDetails := spaceStringsBuilder(string(gotDetailsReport))
				if string(cleanGotDetails) != cleanWantDetails {
					diff := cmp.Diff(cleanWantDetails, cleanGotDetails)
					t.Errorf("details mismatch (-want +got):\n%s", diff)
				}
				//json output on windows is wonky
				if runtime.GOOS != "windows" {
					gotJSON, err := ioutil.ReadFile(outDir + filepath.FromSlash("/results.json"))
					if err != nil {
						t.Errorf("unable to open created report: %v\n", err)
					}
					if string(gotJSON) != tt.wantJSON {
						diff := cmp.Diff(tt.wantJSON, string(gotJSON))
						t.Errorf("json mismatch (-want +got):\n%s", diff)
					}
				}
				os.Remove(filepath.FromSlash("../testdata/summary.html"))
				os.Remove(filepath.FromSlash("../testdata/details.html"))
				os.Remove(filepath.FromSlash("../testdata/results.json"))
			}
		})
	}
}

func TestEnsureHTMLFile(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		wantErr     bool
		wantRewrite bool
	}{
		{
			name:        "validHTMLPath",
			filePath:    filepath.FromSlash("../testdata/testcreate.html"),
			wantErr:     false,
			wantRewrite: false,
		},
		{
			name:        "invalidHTMLFile",
			filePath:    filepath.FromSlash("../testdata/testcreate.txt"),
			wantErr:     false,
			wantRewrite: true,
		},
		{
			name:        "newDir",
			filePath:    filepath.FromSlash("../testdata/newDir/testcreate.html"),
			wantErr:     false,
			wantRewrite: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ensureHTMLFile(tt.filePath)
			if err != nil && !tt.wantErr {
				t.Errorf("ensuring file error: %v", err)
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if tt.wantRewrite {
				dir := filepath.Dir(tt.filePath)
				if _, err := os.Stat(filepath.FromSlash(dir + "/testcreate.html")); err != nil {
					t.Errorf(".html extention replace failed: %v", err)
				}
				os.Remove(filepath.FromSlash(dir + "/testcreate.html"))
			}
			os.Remove(tt.filePath)
			if tt.name == "newDir" {
				os.RemoveAll(filepath.FromSlash("../testdata/newDir"))
			}
		})
	}
}

//spaceStringsBuilder strips all whitespace characters from a string for easier comparison
func spaceStringsBuilder(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}
