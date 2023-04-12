//go:generate go run generator.go

package static

// embedBlob is the object that holds the map of templates in
// a slice of bytes format
type embedBlob struct {
	storage map[string][]byte
}

// internal functions
func newEmbedBlob() *embedBlob {
	return &embedBlob{storage: make(map[string][]byte)}
}

func (e *embedBlob) Add(file string, content []byte) {
	e.storage[file] = content
}

func (e *embedBlob) Get(file string) []byte {
	if f, ok := e.storage[file]; ok {
		return f
	}
	return nil
}

// public interface
var blob = newEmbedBlob()

// Add takes a file and adds it to the blob map
func Add(file string, content []byte) {
	blob.Add(file, content)
}

// Get retrieves a file in the format of a slice of bytes from the blob map
func Get(file string) []byte {
	return blob.Get(file)
}
