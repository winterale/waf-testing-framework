package app

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCheckInvalidChars(t *testing.T) {

	tests := []struct {
		name       string
		Payload    string
		TestType   string
		want       bool
		wantstring string
	}{
		{
			name:       "headerValidator",
			Payload:    "' or user ¶ like'%",
			TestType:   "header",
			want:       true,
			wantstring: "Invalid characters in payload based on RFC 7230: '¶'",
		},
		{
			name:       "cookieValidator",
			Payload:    "-2%20Union%20Select%201,2,3--",
			TestType:   "cookie",
			want:       true,
			wantstring: "Invalid characters in payload based on RFC 2109: ','",
		},
		{
			name:       "pathValidator",
			Payload:    "0^(locate#(0x61,(select id from users where num=1),1)=1)",
			TestType:   "path",
			want:       true,
			wantstring: "Invalid characters in payload based on RFC 3986 and 1738: '#', ' '",
		},
		{
			name:       "queryargValidator",
			Payload:    "0^(locate(0x61,(select id from users where num=1),1)=1)",
			TestType:   "queryarg",
			want:       true,
			wantstring: "Invalid characters in payload based on RFC 3986 and 1738: ' '",
		},
		{
			name:       "DefaultTestcase",
			Payload:    "admin'/*",
			TestType:   "",
			want:       false,
			wantstring: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalidBool, invalidString, _ := checkInvalidChars(tt.Payload, tt.TestType)
			if ok := cmp.Equal(invalidBool, tt.want); !ok {
				diff := cmp.Diff(tt.want, invalidBool)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if ok := cmp.Equal(invalidString, tt.wantstring); !ok {
				diff := cmp.Diff(tt.wantstring, invalidString)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}

}
