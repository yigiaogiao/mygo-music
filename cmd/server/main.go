// Command server is the browser app's other half: it serves MyGo Music
// compiled to WebAssembly, and the music in the folders chosen on the
// machine it runs on.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mygo-music/internal/server"
)

var version = "0.1.31"

func main() {
	addr := flag.String("addr", env("ADDR", ":8090"), "address to listen on")
	data := flag.String("data", env("DATA_DIR", "data"), "directory for the library, covers and lyrics")
	music := flag.String("music", env("MUSIC_DIR", defaultMusicDir()), "folder the setup page browses")
	web := flag.String("web", env("WEB_DIR", "web"), "directory with index.html and app.wasm")
	online := flag.Bool("online-lyrics", envBool("ONLINE_LYRICS", true), "find missing lyrics at LRCLib")
	v := flag.String("version", version, "version to show")
	showVersion := flag.Bool("v", false, "print the version and exit")
	flag.Parse()
	version = *v

	if *showVersion {
		os.Stdout.WriteString(version + "\n")
		return
	}

	srv, err := server.New(server.Config{
		DataDir:      *data,
		MusicDir:     *music,
		WebDir:       *web,
		Version:      version,
		OnlineLyrics: *online,
	})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(*web + "/app.wasm"); err != nil {
		log.Printf("no %s/app.wasm: run scripts/build-web.sh first", *web)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		log.Printf("MyGo Music %s at http://localhost%s", version, *addr)
		log.Printf("music folder %s, data in %s", *music, *data)
		if len(srv.Local().Folders()) == 0 {
			log.Printf("choose folders at http://localhost%s/setup", *addr)
		}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

// defaultMusicDir is the LazyCat netdisk. It is the user's document root,
// which newer runtimes also expose as /lzcapp/document; /lzcapp/media is the
// fallback for a netdisk mounted as media. Local runs set -music instead.
func defaultMusicDir() string {
	first := ""
	for _, dir := range []string{
		"/lzcapp/run/mnt/home",
		"/lzcapp/document",
		"/lzcapp/media/RemoteFS",
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if first == "" {
			first = dir
		}
		if len(entries) > 0 {
			return dir
		}
	}
	if first != "" {
		return first
	}
	return "/lzcapp/media/RemoteFS"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
