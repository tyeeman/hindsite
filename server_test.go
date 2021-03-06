package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func Test_server(t *testing.T) {
	tmpdir := path.Join(os.TempDir(), "hindsite-tests")
	// Initialize temporary directory with test blog.
	os.RemoveAll(tmpdir)
	mkMissingDir(tmpdir)
	site := newSite()
	cmd := "hindsite init " + tmpdir + " -template ./testdata/blog/template"
	args := strings.Split(cmd, " ")
	code := site.executeArgs(args)
	if code != 0 {
		t.Fatalf("%s", cmd)
	}
	if dirCount(path.Join(tmpdir, "template")) != 9 {
		t.Fatalf("%s: unexpected number of riles in template directory", cmd)
	}
	// Start server.
	site = newSite()
	site.out = make(chan string, 100)
	site.in = make(chan string, 1)
	cmd = "hindsite serve " + tmpdir
	args = strings.Split(cmd, " ")
	if err := site.parseArgs(args); err != nil {
		t.Fatalf("serve error: %v", err.Error())
	}
	svr := newServer(&site)
	go func() {
		if err := svr.serve(); err != nil {
			t.Fatalf("serve error: %v", err.Error())
		}
	}()
	waitFor := func(output string) {
		for {
			select {
			case line := <-svr.out:
				line = strings.Replace(line, `\`, `/`, -1) // Normalize MS Windows path separators.
				if strings.Contains(line, output) {
					return
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("%s: timed out waiting for: %v", cmd, output)
				return
			}
		}
	}
	updateAndWait := func(f, text, output string) {
		err := writeFile(f, text)
		if err != nil {
			t.Fatal(err)
		}
		waitFor(output)
	}
	removeAndWait := func(f, output string) {
		err := os.Remove(f)
		if err != nil {
			t.Fatal(err)
		}
		waitFor(output)
	}
	waitFor("Press Ctrl+C to stop")
	// Create new post with copy of existing post.
	existingfile := path.Join(tmpdir, "content", "posts", "2016-10-18-sed-sed.md")
	text, err := readFile(existingfile)
	if err != nil {
		t.Fatal(err)
	}
	newfile := path.Join(tmpdir, "content", "posts", "newfile.md")
	updateAndWait(newfile, text, "error: content/posts/newfile.md: duplicate document build path in: content/posts/2016-10-18-sed-sed.md")
	// Fix post error.
	text = strings.Replace(text, "slug: sed-sed", "slug: newfile", 1)
	updateAndWait(newfile, text, "updated: content/posts/newfile.md")
	// Change post title.
	text = strings.Replace(text, "title: Sed Sed", "title: New File", 1)
	updateAndWait(newfile, text, "updated: content/posts/newfile.md")
	// Add post tag.
	text = strings.Replace(text, "tags: [integer,est,sed,tincidunt]", "tags: [integer,est,sed,tincidunt,newfile]", 1)
	updateAndWait(newfile, text, "updated: content/posts/newfile.md")
	// Remove post.
	removeAndWait(existingfile, "removed: content/posts/2016-10-18-sed-sed.md")
	// Rebuild.
	svr.in <- "R\n"
	waitFor("rebuilding...")
	waitFor("documents: 11")
	// New static file.
	newfile = path.Join(tmpdir, "content", "newfile.txt")
	text = "Hello World!"
	updateAndWait(newfile, text, "updated: content/newfile.txt")
	// Remove static file.
	removeAndWait(newfile, "removed: content/newfile.txt")
	// Stop serve command.
	svr.close(nil)
	time.Sleep(50 * time.Millisecond) // Allow time for serve goroutines to execute cleanup code.
}

// Based onhttps://blog.questionable.services/article/testing-http-handlers-go/
func Test_httpHandlers(t *testing.T) {
	site := newSite()
	site.buildDir = "./testdata/blog/build"
	site.rootConf = newConfig()
	site.rootConf.urlprefix = "http:/example.com"
	svr := newServer(&site)

	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.FileServer(http.Dir(site.buildDir))
	handler = svr.htmlFilter(handler)
	handler = svr.saveBrowserURL(handler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the response body is what we expect.
	if svr.browserURL != "/" {
		t.Errorf("saveBrowserURL handler: url: got %v want %v", svr.browserURL, "/")
	}
	wanted := "<script src=\"http://localhost:35729/livereload.js\"></script>"
	got := rr.Body.String()
	if !strings.Contains(got, wanted) {
		t.Errorf("htmlFilter handler: response did not contain: %#v", wanted)
	}
	if strings.Contains(got, site.rootConf.urlprefix) {
		t.Errorf("htmlFilter handler: response contains urlprefix: %#v", site.rootConf.urlprefix)
	}
}
