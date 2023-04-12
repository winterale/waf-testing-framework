package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var testYaml = filepath.FromSlash("../testdata/testyaml.yml")
var testDataPayloads = filepath.FromSlash("../testdata/payloads")

var defaultTestBlock = FileTestBlock{
	Name: "Defaults",
}

var customTestBlock = FileTestBlock{
	Name:     "Custom",
	Protocol: "HTTPS",
	Host:     "10.10.10.10",
	Port:     4000,
	Path:     "/foobar",
	DefaultHeaders: []*Header{
		{
			Header: "Content-Type",
			Value:  "application/json",
		},
		{
			Header: "User-Agent",
			Value:  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.162 Safari/537.36",
		},
	},
	AllowCondition: &Condition{
		Code: 201,
		Headers: []*Header{
			{
				Header: "Foo",
				Value:  "bar",
			},
			{
				Header: "Lorem",
				Value:  "Ipsum",
			},
		},
	},
	BlockCondition: &Condition{
		Code: 403,
		Headers: []*Header{
			{
				Header: "Foo",
				Value:  "bar",
			},
			{
				Header: "Lorem",
				Value:  "Ipsum",
			},
		},
	},
}

var fnTestFile = TestFile{
	File:     filepath.FromSlash("../testdata/payloads/false_negatives/fn.txt"),
	TestType: "falseNegative",
}

var fpTestFile = TestFile{
	File:     filepath.FromSlash("../testdata/payloads/false_positives/fp.txt"),
	TestType: "falsePositive",
}

var defaultTestSet = &TestSet{
	Name: "Defaults",
	URI:  "http://localhost:80/",
	DefaultHeaders: map[string][]string{
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Accept-Encoding": {"gzip, deflate"},
		"Cache-Control":   {"max-age=0"},
		"Connection":      {"close"},
		"Content-Type":    {"application/x-www-form-urlencoded"},
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_4) AppleWebKit/600.7.12 (KHTML, like Gecko) Version/8.0.7 Safari/600.7.12"},
	},
	AllowCondition: &Condition{
		Code:    0,
		Headers: nil,
	},
	BlockCondition: &Condition{
		Code:    406,
		Headers: nil,
	},
}

var customTestSet = &TestSet{
	Name: "Custom",
	URI:  "https://10.10.10.10:4000/foobar",
	DefaultHeaders: map[string][]string{
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Accept-Encoding": {"gzip, deflate"},
		"Cache-Control":   {"max-age=0"},
		"Connection":      {"close"},
		"Content-Type":    {"application/json"},
		"User-Agent":      {"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.162 Safari/537.36"},
	},
	AllowCondition: &Condition{
		Code:    0,
		Headers: []*Header{{Header: "Foo", Value: "bar"}, {Header: "Lorem", Value: "Ipsum"}},
	},
	BlockCondition: &Condition{
		Code:    403,
		Headers: []*Header{{Header: "Foo", Value: "bar"}, {Header: "Lorem", Value: "Ipsum"}},
	},
}
var testFile = &File{
	Tests:            []*FileTestBlock{&defaultTestBlock, &customTestBlock},
	PayloadDir:       "../testdata/payloads",
	PayloadLocations: []*TestLocation{{Location: "body", Key: "Bar"}, {Location: "header", Key: "foobar"}},
}

var defaultTestFile = &File{
	Tests:      []*FileTestBlock{&defaultTestBlock},
	PayloadDir: filepath.FromSlash("../testdata/payloads"),
}

var testRun = &TestRun{
	TestSets:     []*TestSet{defaultTestSet, customTestSet},
	PayloadDir:   testDataPayloads,
	TestFiles:    []*TestFile{&fnTestFile, &fpTestFile},
	PostBodyType: "raw",
	Locations: []*TestLocation{
		{
			Location: "body",
			Key:      "Bar",
		},
		{
			Location: "header",
			Key:      "foobar",
		},
	},
}
var defaultTestRun = &TestRun{
	TestSets:     []*TestSet{defaultTestSet},
	PayloadDir:   testDataPayloads,
	TestFiles:    []*TestFile{&fnTestFile, &fpTestFile},
	PostBodyType: "raw",
	Locations: []*TestLocation{
		{
			Location: "header",
			Key:      "foo",
		},
		{
			Location: "path",
		},
		{
			Location: "queryarg",
			Key:      "foo",
		},
		{
			Location: "cookie",
			Key:      "foo",
		},
		{
			Location: "body",
			Key:      "foo",
		},
	},
}

func TestParseYamlFile(t *testing.T) {

	file, err := os.Open(testYaml)
	if err != nil {
		t.Fatalf("unable to open test yaml file: %v", err)
	}

	tests := []struct {
		name   string
		reader io.Reader
		want   *File
	}{
		{
			name:   "valid yaml",
			reader: file,
			want:   testFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := ParseYamlFile(tt.reader)
			if ok := cmp.Equal(out, tt.want); !ok {
				diff := cmp.Diff(tt.want, out)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseConfigs(t *testing.T) {

	tests := []struct {
		name string
		file *File
		want *TestRun
	}{
		{
			name: "parseConfigs",
			file: testFile,
			want: testRun,
		},
		{
			name: "deafultParseConfigs",
			file: defaultTestFile,
			want: defaultTestRun,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ParseConfigs(tt.file)
			if err != nil {
				t.Error(err)
			}
			if ok := cmp.Equal(out, tt.want); !ok {
				diff := cmp.Diff(tt.want, out)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

var testFiles = []*TestFile{
	{
		File:     filepath.FromSlash("../testdata/payloads/false_negatives/fn.txt"),
		TestType: "falseNegative"},
	{
		File:     filepath.FromSlash("../testdata/payloads/false_positives/fp.txt"),
		TestType: "falsePositive",
	},
}

func TestWalkFiles(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		want    []*TestFile
		wantErr bool
	}{
		{
			name:    "testbuilder",
			root:    testDataPayloads,
			want:    testFiles,
			wantErr: false,
		},
		{
			name:    "badDir",
			root:    filepath.FromSlash("/does/not/exist"),
			wantErr: true,
		},
		{
			name:    "unknownDir",
			root:    testDataPayloads,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "unknownDir" {
				os.Mkdir(filepath.FromSlash("../testdata/payloads/bad_dir"), os.ModePerm)
				os.Create(filepath.FromSlash("../testdata/payloads/bad_dir/test.txt"))
			}
			out, err := walkFiles(tt.root)
			if err != nil && !tt.wantErr {
				t.Errorf("%v", err)
			}
			if err != nil && tt.wantErr {
			}
			if err == nil && tt.wantErr {
				t.Errorf("no expected error")
			}
			if err == nil && !tt.wantErr {
				if ok := cmp.Equal(out, tt.want); !ok {
					diff := cmp.Diff(tt.want, out)
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
			os.RemoveAll(filepath.FromSlash("../testdata/payloads/bad_dir"))
		})
	}
}
