package app

import (
	"strings"

	"regexp"
)

//InvalidRequest struct
type InvalidRequest struct {
	Request      *TestRequest
	InvalidChars string
	InvalidIndex []int
}

// Apply the regular expression and return the list of all sub-matches
// and a list of the positions. The positions are unique, and calculated
// doing an average of the positions of all sub-matches.
func reList(re *regexp.Regexp, Payload string) ([][][]byte, []int) {
	matchs := re.FindAllSubmatch([]byte(Payload), -1)
	pos := re.FindAllStringSubmatchIndex(Payload, -1)

	// Merge positions into a single value (the start one)
	newpos := make([]int, len(pos))
	for i, p := range pos {
		sum := 0
		items := 0
		for _, n := range p {
			sum += n
			items++
		}
		newpos[i] = sum / items
	}
	return matchs, newpos
}

//checkInvalidChars checks the payload to see if there are invalid characters within the payload.
//It returns a boolean indicating if invalid characters were found, the invalid charcters, and where
//in the payload the invalid charcters were found
func checkInvalidChars(Payload string, TestType string) (bool, string, []int) {
	invalidString := "Invalid characters in payload "
	var invalidCharsResult []string
	switch TestType {
	//RFC 7230
	case "header":
		matched, _ := regexp.Match(`[^\x20-\x7E]+`, []byte(Payload))
		if matched {
			payloadRegex, _ := regexp.Compile(`[^\x20-\x7E]+`)
			invalidChars, invalidIndex := reList(payloadRegex, Payload)
			//append all chars of [][][] byte array to single string
			for _, line := range invalidChars {
				for _, match := range line { // match is a type of []byte
					if !stringContains(invalidCharsResult, string(match)) {
						invalidCharsResult = append(invalidCharsResult, string(match))
					}
				}
			}
			invalidString += "based on RFC 7230: "
			for _, char := range invalidCharsResult {
				invalidString += "'" + char + "', "
			}
			invalidString = strings.TrimSuffix(invalidString, ", ")
			return true, invalidString, invalidIndex
		}
		return false, "", nil
	//RFC 3986 & RFC 1738
	//valid characters are a-z A-Z 0-9 . - _ ~ ! $ & ' ( ) * + , ; = : @ %
	case "path", "queryarg":
		matched, _ := regexp.Match(`[^!\x24-\x7E]+`, []byte(Payload))
		if matched {
			payloadRegex, _ := regexp.Compile(`[^!\x24-\x7E]+`)
			invalidChars, invalidIndex := reList(payloadRegex, Payload)
			//append all chars of [][][] byte array to single string
			for _, line := range invalidChars {
				for _, match := range line { // match is a type of []byte
					if !stringContains(invalidCharsResult, string(match)) {
						invalidCharsResult = append(invalidCharsResult, string(match))
					}
				}
			}
			invalidString += "based on RFC 3986 and 1738: "
			for _, char := range invalidCharsResult {
				invalidString += "'" + char + "', "
			}
			invalidString = strings.TrimSuffix(invalidString, ", ")
			return true, invalidString, invalidIndex
		}
		return false, "", nil
	// RFC 2109
	//alphanum + !#$%&'()*+-./:<=>?@[]^_`{|}~
	case "cookie":
		matched, _ := regexp.Match(`[^!\x23-\x2B\x2D-\x3A\x3C-\x5B\x5D-\x7E]`, []byte(Payload))
		if matched {
			payloadRegex, _ := regexp.Compile(`[^!\x23-\x2B\x2D-\x3A\x3C-\x5B\x5D-\x7E]`)
			invalidChars, invalidIndex := reList(payloadRegex, Payload)
			//append all chars of [][][] byte array to single string
			for _, line := range invalidChars {
				for _, match := range line { // match is a type of []byte
					if !stringContains(invalidCharsResult, string(match)) {
						invalidCharsResult = append(invalidCharsResult, string(match))
					}
				}
			}
			invalidString += "based on RFC 2109: "
			for _, char := range invalidCharsResult {
				invalidString += "'" + char + "', "
			}
			invalidString = strings.TrimSuffix(invalidString, ", ")
			return true, invalidString, invalidIndex
		}
		return false, "", nil
	default:
		return false, "", nil
	}
}
