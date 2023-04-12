package results

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/signalsciences/waf-testing-framework/pkg/config"
	"github.com/signalsciences/waf-testing-framework/pkg/static"
)

//TestResult is the individual test level result object
type TestResult struct {
	SetName  string `json:"-"`
	FileName string `json:"-"`
	Line     int    `json:"-"`
	Payload  string `json:"-"`
	Location string `json:"-"`

	Outcome  string
	Request  string
	Response string
}

//FileResult is the file level result object
type FileResult struct {
	FailedLines    []int `json:"-"`
	PayloadResults map[int]*PayloadResult
}

//PayloadResult is the payload level result object
type PayloadResult struct {
	Line       int
	Payload    string
	SetResults map[string]*SetResult
}

//SetResult is the set level result object
type SetResult struct {
	Locations map[string]*TestResult
}

//SetCounts is the object that stores the numeric counts fort the results of a test
type SetCounts struct {
	FpCount          int
	FpPercent        float64
	FnCount          int
	FnPercent        float64
	InvCount         int
	ErrCount         int
	PassedCount      int
	FailPercent      float64
	TotalFPTestCount int
	TotalFNTestCount int
	TotalCount       int
}

//Results is the top level result object
type Results struct {
	StartTime   string
	EndTime     string
	Config      *config.TestRun
	SetCounts   map[string]*SetCounts
	FileResults map[string]*FileResult
	Reporting   *Reporting
}

//Reporting is the object defining where reporting templates and outputs are located
type Reporting struct {
	OutputDir string
}

//OverallReport is the object passed to generate the HTML report
type OverallReport struct {
	TestSets  []string
	Locations []string
	Matrix    []*FileReport
	Config    string
	Results   *Results
}

//RowReport is the object that stores a row in the comparison matrix
type RowReport struct {
	Line      int
	Payload   string
	SetReport map[string][]int
}

//FileReport is the object that stores the file results in the matrix
type FileReport struct {
	FileName  string
	RowReport []*RowReport
}

//InitResults builds an empty result object to be used in the testing
func InitResults(testRun *config.TestRun) *Results {
	fileResults := make(map[string]*FileResult)
	for _, fileTest := range testRun.TestFiles {
		parts := strings.Split(fileTest.File, string(os.PathSeparator))
		parentDir := parts[len(parts)-2]
		fileName := parentDir + string(os.PathSeparator) + filepath.Base(fileTest.File)

		fileResults[fileName] = &FileResult{}
		fileResults[fileName].PayloadResults = make(map[int]*PayloadResult)
	}
	var setCounts = make(map[string]*SetCounts)
	for _, testSet := range testRun.TestSets {
		setCounts[testSet.Name] = &SetCounts{}
	}
	results := &Results{
		StartTime:   time.Now().Local().Format("02 Jan 2006, 15:04 MST"),
		Config:      testRun,
		FileResults: fileResults,
		SetCounts:   setCounts,
	}
	//define reporting
	results.Reporting = &Reporting{
		OutputDir: filepath.FromSlash("./output"),
	}
	return results
}

//ProcessResults calculates the percentage of each test outcome with a decimal precision of 2 and stores
//it in the reuslt object. It also calculates the total number of passed tests at each result level.
func (r *Results) ProcessResults() {
	for _, setCount := range r.SetCounts {
		setCount.TotalCount = setCount.TotalFNTestCount + setCount.TotalFPTestCount
		//setCount.PassedCount = setCount.TotalCount - setCount.FpCount - setCount.FnCount - setCount.InvCount - setCount.ErrCount
		if setCount.FpCount != 0 {
			setCount.FpPercent = math.Round(float64(setCount.FpCount)/float64(setCount.TotalFPTestCount)*10000) / 100
		} else {
			setCount.FpPercent = 0.00
		}
		if setCount.FnCount != 0 {
			setCount.FnPercent = math.Round(float64(setCount.FnCount)/float64(setCount.TotalFNTestCount)*10000) / 100
		} else {
			setCount.FnPercent = 0.00
		}
		setCount.FailPercent = math.Round((float64(setCount.FnCount)+float64(setCount.FpCount))/float64(setCount.TotalFNTestCount+setCount.TotalFPTestCount)*10000) / 100
	}
	r.EndTime = time.Now().Local().Format("02 Jan 2006, 15:04 MST")
}

//ReportData takes in the results of the testruns and modifies the structure to be
//used in the generation of the matrix in the HTML report
func (r *Results) ReportData() *OverallReport {
	var locations, testSets []string
	//get locations
	for _, loc := range r.Config.Locations {
		locations = append(locations, loc.Location)
	}
	//get testSets
	for _, testSet := range r.Config.TestSets {
		testSets = append(testSets, testSet.Name)
	}
	sort.Strings(locations)
	sort.Strings(testSets)
	//get setResults
	var matrix []*FileReport
	for fileName, fileResult := range r.FileResults {
		var rowReports []*RowReport
		//sort to get ordered output
		sort.Ints(fileResult.FailedLines)
		for _, line := range fileResult.FailedLines {
			//skip lines with no failures in any test
			if fileResult.PayloadResults[line] != nil {
				payloadResult := fileResult.PayloadResults[line]
				var setReport = make(map[string][]int)
				//use testSets instead of payloadResults.SetResults to guarantee order
				for _, setName := range testSets {
					var t []int
					//if there are no results, the test set had no failures for this line
					if payloadResult.SetResults[setName] == nil {
						for i := 0; i < len(locations); i++ {
							t = append(t, 0)
						}
					} else {
						//use locations instead of setResult.Locations to guarantee order
						for _, location := range locations {
							locationResult := payloadResult.SetResults[setName].Locations[location]
							if locationResult == nil {
								t = append(t, 0)
							} else if locationResult.Outcome == "falseNegative" || locationResult.Outcome == "falsePositive" {
								t = append(t, 1)
							} else if locationResult.Outcome == "invalid" {
								t = append(t, 2)
							} else if locationResult.Outcome == "error" {
								t = append(t, 3)
							}
						}
					}
					setReport[setName] = t
				}
				rowReport := &RowReport{
					Line:      payloadResult.Line,
					Payload:   payloadResult.Payload,
					SetReport: setReport,
				}
				rowReports = append(rowReports, rowReport)

			}
		}
		fileReport := &FileReport{
			FileName:  fileName,
			RowReport: rowReports,
		}
		matrix = append(matrix, fileReport)
	}
	prettyConfig, _ := json.MarshalIndent(r.Config, "", "  ")

	return &OverallReport{
		TestSets:  testSets,
		Locations: locations,
		Matrix:    matrix,
		Config:    string(prettyConfig),
		Results:   r,
	}
}

//GenerateReports generates a summary and a details HTML report of the results of the results.
//It also generates a json report of the results.
func (r *Results) GenerateReports() error {
	//check the directory exists
	if _, serr := os.Stat(r.Reporting.OutputDir); serr != nil {
		merr := os.MkdirAll(r.Reporting.OutputDir, os.ModePerm)
		if merr != nil {
			return merr
		}
	}
	//ouput results as json
	resultout, _ := json.MarshalIndent(r, "", "  ")
	jsonOut := filepath.FromSlash(r.Reporting.OutputDir + "/results.json")
	err := ioutil.WriteFile(jsonOut, resultout, 0644)
	if err != nil {
		fmt.Printf("unable to write results to json: %v\n", err)
		log.Fatalf("unable to write results to json: %v", err)
	}
	templateData := struct {
		Report *OverallReport
	}{
		Report: r.ReportData(),
	}
	funcMap := template.FuncMap{
		"grade": func(per float64) string {
			if per <= 1.00 {
				return "A"
			} else if per >= 1.00 && per <= 2.00 {
				return "B"
			} else if per >= 2.00 && per <= 3.00 {
				return "C"
			} else if per >= 3.00 && per <= 4.00 {
				return "D"
			} else {
				return "F"
			}
		},
		"mul": func(per float64) float64 {
			return math.Max(0.5, math.Min(per/5*100, 100.00))
		},
	}
	//get the template for the results
	summary := string(static.Get("/summary.tmpl"))
	t, err := template.New("summary.tmpl").Funcs(funcMap).Parse(summary)
	if err != nil {
		return fmt.Errorf("unable to open template file: %v", err)
	}
	//make sure the results file can be created
	out, err := ensureHTMLFile(filepath.FromSlash(r.Reporting.OutputDir + "/summary.html")) //r.Config.HTMLResultsFile
	if err != nil {
		return err
	}
	err = t.Execute(out, templateData)
	if err != nil {
		return err
	}
	out.Close()
	//generate details report
	details := string(static.Get("/details.tmpl"))
	t, err = template.New("details.tmpl").Funcs(funcMap).Parse(details)
	if err != nil {
		return fmt.Errorf("unable to open template file: %v", err)
	}
	out, err = ensureHTMLFile(filepath.FromSlash(r.Reporting.OutputDir + "/details.html"))
	if err != nil {
		return err
	}
	err = t.Execute(out, templateData)
	if err != nil {
		return err
	}
	out.Close()
	return nil
}

//ensureHTMLFile looks to see if the directory in a filepath exists and if not
//attempts to create it before creating the file.
func ensureHTMLFile(outPath string) (*os.File, error) {
	dirName := filepath.Dir(outPath)
	//check to make sure the file to be created is html. If not, replace the provided
	// file extension with .html
	if fileExt := strings.ToLower(filepath.Ext(outPath)); fileExt != ".html" {
		newPath := strings.TrimSuffix(outPath, filepath.Ext(outPath))
		outPath = newPath + ".html"
	}
	//check the directory exists
	if _, serr := os.Stat(dirName); serr != nil {
		merr := os.MkdirAll(dirName, os.ModePerm)
		if merr != nil {
			return nil, merr
		}
	}
	out, cerr := os.Create(outPath)
	if cerr != nil {
		return nil, cerr
	}
	return out, nil
}
