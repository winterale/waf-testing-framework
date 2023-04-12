package config

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

//Header is the struct that holds header kv pairs
type Header struct {
	Header string `yaml:"header"`
	Value  string `yaml:"value"`
}

//Condition is the struct that holds conditions for allow/block responses
type Condition struct {
	Code    int       `yaml:"code"`
	Headers []*Header `yaml:"headers"`
}

//FileTestBlock represents a test set from the yaml file
type FileTestBlock struct {
	Name           string     `yaml:"name"`
	Protocol       string     `yaml:"protocol"`
	Host           string     `yaml:"host"`
	Port           int        `yaml:"port"`
	Path           string     `yaml:"path"`
	DefaultHeaders []*Header  `yaml:"default_headers"`
	AllowCondition *Condition `yaml:"allow_condition"`
	BlockCondition *Condition `yaml:"block_condition"`
}

//File is the object that represents the .yaml config file
type File struct {
	Tests            []*FileTestBlock `yaml:"wafs"`
	PayloadDir       string           `yaml:"payload_dir"`
	PostBodyType     string           `yaml:"postbody_type"`
	URLEncodePath    bool             `yaml:"urlencode_path"`
	URLEncodeQuery   bool             `yaml:"urlencode_query"`
	URLENcodeHeader  bool             `yaml:"urlencode_header"`
	B64EncodeCookie  bool             `yaml:"b64encode_cookie"`
	PayloadLocations []*TestLocation  `yaml:"payload_locations"`
}

//TestRun is the object that hold the configurations for a full test set being run
type TestRun struct {
	PayloadDir      string
	URLEncodePath   bool
	URLEncodeQuery  bool
	URLEncodeHeader bool
	B64EncodeCookie bool
	PostBodyType    string
	Locations       []*TestLocation
	TestFiles       []*TestFile `json:"-"`
	TestSets        []*TestSet
}

//TestSet is the object that represents and individual testset settings
type TestSet struct {
	Name           string
	URI            string
	DefaultHeaders map[string][]string
	AllowCondition *Condition
	BlockCondition *Condition
}

//TestFile is the object that holds a file that contains tests
type TestFile struct {
	File     string
	TestType string
}

//TestLocation represents a single test location
type TestLocation struct {
	Location string `yaml:"location"`
	Key      string `yaml:"key" json:",omitempty"`
}

//ParseYamlFile parses the given .yaml config into the File object
func ParseYamlFile(r io.Reader) (*File, error) {
	yamlFile, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var f File
	err = yaml.Unmarshal(yamlFile, &f)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

//ParseConfigs prases the File object into the TestRun object, applying defaults where necessary
func ParseConfigs(file *File) (*TestRun, error) {
	var testRun = TestRun{}
	//test locations
	var locations []*TestLocation
	if len(file.PayloadLocations) == 0 {
		headerLocation := &TestLocation{
			Location: "header",
			Key:      "foo",
		}
		locations = append(locations, headerLocation)
		pathLocation := &TestLocation{
			Location: "path",
		}
		locations = append(locations, pathLocation)
		queryargLocation := &TestLocation{
			Location: "queryarg",
			Key:      "foo",
		}
		locations = append(locations, queryargLocation)
		cookieLocation := &TestLocation{
			Location: "cookie",
			Key:      "foo",
		}
		locations = append(locations, cookieLocation)
		bodyLocation := &TestLocation{
			Location: "body",
			Key:      "foo",
		}
		locations = append(locations, bodyLocation)
	} else {
		for _, l := range file.PayloadLocations {
			location := &TestLocation{
				Location: l.Location,
				Key:      l.Key,
			}
			locations = append(locations, location)
		}
	}
	testRun.Locations = locations
	//postbody type
	if file.PostBodyType == "" {
		file.PostBodyType = "raw"
	}
	testRun.PostBodyType = file.PostBodyType
	//url encode path
	testRun.URLEncodePath = file.URLEncodePath
	//url encode query
	testRun.URLEncodeQuery = file.URLEncodeQuery
	//urlencode query
	testRun.URLEncodeHeader = file.URLENcodeHeader
	//base64 encode cookie
	testRun.B64EncodeCookie = file.B64EncodeCookie
	//payload directory
	if file.PayloadDir == "" {
		file.PayloadDir = "payloads"
	}
	testRun.PayloadDir = filepath.FromSlash(file.PayloadDir)
	//payloads
	testFiles, err := walkFiles(file.PayloadDir)
	if err != nil {
		return nil, err
	}
	testRun.TestFiles = testFiles
	for _, testDef := range file.Tests {
		//protocol
		if testDef.Protocol == "" {
			testDef.Protocol = "http"
		} else {
			testDef.Protocol = strings.ToLower(testDef.Protocol)
		}
		//host
		if testDef.Host == "" {
			testDef.Host = "localhost"
		}
		//port
		if testDef.Port == 0 {
			testDef.Port = 80
		}
		//path
		if testDef.Path != "" {
			testDef.Path = filepath.Clean(testDef.Path)
		}
		testDef.Path = strings.TrimLeft(testDef.Path, string(os.PathSeparator))
		//headers
		headers := map[string][]string{
			"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			"Accept-Encoding": {"gzip, deflate"},
			"Connection":      {"close"},
			"Content-Type":    {"application/x-www-form-urlencoded"},
			"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_4) AppleWebKit/600.7.12 (KHTML, like Gecko) Version/8.0.7 Safari/600.7.12"},
			"Cache-Control":   {"max-age=0"},
		}
		//add defined headers by replacing header if it exists
		for _, h := range testDef.DefaultHeaders {
			key := http.CanonicalHeaderKey(h.Header)
			delete(headers, key)
			headers[key] = []string{h.Value}
		}
		//block test conditions
		if testDef.BlockCondition == nil {
			testDef.BlockCondition = &Condition{
				Code:    406,
				Headers: nil,
			}
		}
		var blockHeaders []*Header
		if testDef.BlockCondition.Headers != nil {
			for _, h := range testDef.BlockCondition.Headers {
				blockHeader := &Header{
					Header: h.Header,
					Value:  h.Value,
				}
				blockHeaders = append(blockHeaders, blockHeader)
			}
		}
		blockConditon := &Condition{
			Code:    testDef.BlockCondition.Code,
			Headers: blockHeaders,
		}
		//allow test conditions
		var allowHeaders []*Header
		if testDef.AllowCondition != nil {
			if testDef.AllowCondition.Headers != nil {
				for _, h := range testDef.AllowCondition.Headers {
					allowHeader := &Header{
						Header: h.Header,
						Value:  h.Value,
					}
					allowHeaders = append(allowHeaders, allowHeader)
				}
			}
		}
		allowConditon := &Condition{
			Headers: allowHeaders,
		}
		//create the TestSet config object
		testSet := &TestSet{
			Name:           testDef.Name,
			URI:            fmt.Sprintf("%s://%s:%s/%s", testDef.Protocol, testDef.Host, strconv.Itoa(testDef.Port), testDef.Path),
			DefaultHeaders: headers,
			AllowCondition: allowConditon,
			BlockCondition: blockConditon,
		}
		testRun.TestSets = append(testRun.TestSets, testSet)
	}
	return &testRun, nil
}

// walkFiles starts a goroutine to walk the directory tree at root and send the
// path of each regular file on the string channel.  It sends the result of the
// walk on the error channel.  If done is closed, walkFiles abandons its work.
func walkFiles(root string) ([]*TestFile, error) {
	var files []*TestFile
	// No select needed for this send, since errc is buffered.
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		//skip files beginning with "." ex: .DS_Store
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		var testType string
		if strings.Contains(path, "false_positive") {
			testType = "falsePositive"
		} else if strings.Contains(path, "false_negative") {
			testType = "falseNegative"
		} else {
			return fmt.Errorf("unknown test directory type: %v", path)
		}
		file := &TestFile{
			File:     path,
			TestType: testType,
		}
		files = append(files, file)
		return nil
	})
	return files, err
}
