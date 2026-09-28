// Command map serves the static Explore demo.
//
// Run from the repository root, after the wasm module is built into this directory:
//
//	./examples/wasm/build.sh examples/map/search.wasm
//	go run ./examples/map
package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dir := "examples/map"
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		log.Fatal("run from the repository root: go run ./examples/map")
	}
	if err := mime.AddExtensionType(".wasm", "application/wasm"); err != nil {
		log.Fatal(err)
	}
	addr := "127.0.0.1:8080"
	log.Printf("http://%s", addr)
	if err := http.ListenAndServe(addr, http.FileServer(http.Dir(dir))); err != nil {
		log.Fatal(err)
	}
}
