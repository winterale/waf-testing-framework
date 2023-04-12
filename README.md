This repository is a clone of one from Signal Sciences which is no longer available. 
# WAF Testing Framework
WAF Testing Framework is a tool designed to test the effectiveness of WAF and RASP tools in their ability to filter request traffic between an HTTP/S client and server. Test payloads are taken from text files in a specified directory, transformed to HTTP-requests, and sent to specified address (IP or hostname). Results are recorded and presented in both JSON and HTML report formats.

## Running the tool
#### From precompiled binaries
Visit the releases page for precompiled binaries. There are zip files for windows, mac, and linux. Inside each zip file is the binary to run as well as the payloads directory and tool configuration yaml file.

#### From source
The tool requires golang 1.14 [(install guides here)](https://golang.org/doc/install)
```
go build -o waftf cmd/main.go
```

Executing the binary will run the tool with all default options and flags, and generate a summary report at `output/sumamry.html`, a details report at `output/details.html`, and the raw JSON results at `output/results.json`. A runtime log will be created at `output/runtime.log` and an error log will be created at `output/error.log`. Customization options are provided below.

#### Interpreting the results
The reports will only display the results of a tested payload when it either fails, is invalid, or causes an error for at least one test location. This means that if a tested payload is correctly handled in all locations, it will not appear in the reports.

## Test Payloads
Payloads to be tested can be defined in a specified directory (default is `payloads`). The directory must follow the following structure:
```
<dir_name>
    ├──false_negatives
    |   ├── <file_name>.txt
    |   ├── ...
    ├──false_positives
        ├── <file_name>.txt
        ├── ...
```

Files must be `.txt` files and have one payload per line. Files located under `false_negatives` will be run as tests looking for false negatives and files located under `false_postitives` will be run as tests looking for false positives. **Payloads are sent as-is**, they are not automatically encoded or modified in any way unless specified with options.

## Default options
```
URL:                http://localhost:80
Payload directory:  ./payloads
Allow Condition:    not response code 406
Block Condition:    response code 406
Default Headers:    Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
		    Accept-Encoding: gzip, deflate
		    Connection: close
		    Content-Type: application/x-www-form-urlencoded
		    User-Agent: {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_4) AppleWebKit/600.7.12 (KHTML, like Gecko) Version/8.0.7 Safari/600.7.12"},
		    Cache-Control: max-age=0
Test Locations:     Header, Path, Query Argument, Cookie, Post Body
```

## YAML file
The configuration yaml file allows you to specify options to change the behavior of the tool. The yaml file options are defined below:
```
---
urlencode_path:       <true/false>    boolean to determine if payloads sent in the path of a request should be URL encoded
urlencode_query:      <true/false>    boolean to determine if payloads sent in the query of a request should be URL encoded
urlencode_header:     <true/false>    boolean to determine if payloads sent in the header of a request should be URL encoded
b64encode_cookie      <true/false>    boolean to determine if payloads sent in the cookie of a request should be base64 encoded
postbody_type         <string>        format of the payload for the post body (raw, urlencoded, json)
payload_dir:          <path>          (required) directory in which the test flies are located
payload_locations:                    (required) list of where payloads should be run
  - location:         <string>        (required) body, header, path, queryarg, cookie
    key:              <string>        (required) the parameter value the payload will be assigned to. Not required for path
wafs:                                 list of WAFs to run tests against, defined by their name
  - name:             <string>        (required) name of the WAF
    protocol:         <string>        (required) protocol for the requests
    host:             <string>        (required) hostname to send requests to
    port:             <number>        (required) port to send requests to
    path:             <string>        path to send requests to
    default_headers:                  list of headers to be added to every test request
      - header:       <string>
        value:        <string>
    block_condition:                  conditions which indicate a block by the WAF
      code:           <number>        (required) HTTP response code
      headers:                        list of header values added in WAF response that indicate a block decision
        - header:     <string>
          value:      <string>
    allow_condition:                  conditions which indicate an allow by the WAF
      headers:                        list of header values added in WAF response that indicate an allow decision
        - header:     <string>
          value:      <string>
```

## Option flags
There are a number of option flags you can pass to the binary
```
-config, -c     <path>        path to the yaml config file. DEFAULT: ./config.yaml
-debug, -d      <true/false>  set the log level to debug. DEFAULT false
-processor, -p  <number>      the maximum number of operating system threads (CPUs) that will be
                              used to execute the testing tool simultaneously. DEFAULT: maximum for your system
-rate, -r       <number>      set the maximum number of requests per second generated during the test. DEFAULT: 50
-worker, -w     <number>      set the maximum number of workers to concurrently send requests and process
                              results. DEFAULT: 10
-version, -v                  prints the current version of the tool
```

## Sample YAML file
```
---
urlencode_path: true
urlencode_query: true
b64encode_cookie: true
postbody_type: urlencoded
payload_dir: payloads
payload_locations:
  - location: body
    key: foobar
  - location: header
    key: Bar
  - location: cookie
    key: foobar
  - location: path
  - location: queryarg
    key: Bar
wafs:
  - name: WAF1
    protocol: HTTP
    host: localhost
    port: 80
    path:
    block_condition:
      code: 406
  - name: WAF2
    protocol: HTTP
    host: anotherhost
    port: 80
    path:
    block_condition:
      code: 406
```
