package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"nickcast/config"
	"nickcast/internal/NickServAuth"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// maxRingBufferSize determines how much recent audio data to buffer for new listeners.
	// This should be enough to satisfy a player's initial buffering requirements.
	// A few kilobytes (e.g., 64KB, 128KB, 256KB) is often a good starting point for MP3.
	// You might need to tune this based on your audio bitrate and player.
	maxRingBufferSize = 128 * 1024 // 128 KB
)

var (
	listeners   = make(map[chan []byte]struct{})
	listenersMu sync.Mutex

	firstData     chan struct{} // Closed when the first stream data is received.
	firstDataOnce sync.Once     // Ensures firstData is closed only once per stream session.

	streamActive atomic.Bool // Atomic boolean to indicate if a streamer is actively sending data.

	streamCancelFn context.CancelFunc // Function to cancel the context for active listeners.
	streamCtx      context.Context    // The context for the current stream.
	streamCtxMu    sync.Mutex         // Protects streamCtx and streamCancelFn

	// ringBuffer stores the most recent audio data for new listeners.
	ringBuffer    *bytes.Buffer
	ringBufferMu  sync.Mutex
)

func Start() {
	// Initialize firstData channel and ring buffer at startup
	resetStreamState()

	http.HandleFunc("/stream", streamHandler)
	http.HandleFunc("/listen", listenHandler)
	log.Printf("Listening on %s", config.AppConfig.ListenAddress)
	log.Fatal(http.ListenAndServe(config.AppConfig.ListenAddress, nil))
}

// resetStreamState resets the channels and buffers for a new stream session.
// This should be called when a new stream is expected to start.
func resetStreamState() {
	firstDataOnce = sync.Once{}
	firstData = make(chan struct{})

	ringBufferMu.Lock()
	ringBuffer = bytes.NewBuffer(make([]byte, 0, maxRingBufferSize)) // Initialize with capacity
	ringBufferMu.Unlock()

	// Ensure streamCtx and streamCancelFn are initialized for immediate use
	// even before a streamer connects, to avoid nil pointer issues.
	streamCtxMu.Lock()
	if streamCancelFn != nil {
		streamCancelFn() // Cancel any existing context
	}
	streamCtx, streamCancelFn = context.WithCancel(context.Background())
	streamCtxMu.Unlock()
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	// Only one streamer at a time. If another streamer tries to connect, reject.
	if !streamActive.CompareAndSwap(false, true) {
		log.Printf("Another streamer tried to connect from %s, but a stream is already active.", r.RemoteAddr)
		http.Error(w, "Stream already active", http.StatusConflict)
		return
	}

	user, pass, ok := parseBasicAuth(r)
	if !ok {
		sourcePass := r.Header.Get("X-Source-Password")
		if sourcePass == "" {
			sourcePass = r.URL.Query().Get("password")
		}
		if sourcePass != "" {
			parts := strings.SplitN(sourcePass, ":", 2)
			if len(parts) == 2 {
				user, pass, ok = parts[0], parts[1], true
			}
		}
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="NickStream"`)
			http.Error(w, "Unauthorized - no credentials", http.StatusUnauthorized)
			streamActive.Store(false) // Release stream lock
			return
		}
	}

	auth := NickServAuth.NewAuthClient(config.AppConfig.AuthURL, config.AppConfig.APIToken)
	valid, err := auth.Authenticate(user, pass)
	if err != nil || !valid {
		log.Printf("Auth failed for user %s from %s: %v", user, r.RemoteAddr, err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		streamActive.Store(false) // Release stream lock
		return
	}

	log.Printf("Streamer %s connected from %s", user, r.RemoteAddr)

	// Set up new stream context for listeners
	streamCtxMu.Lock()
	if streamCancelFn != nil { // Cancel previous context if it exists
		streamCancelFn()
	}
	streamCtx, streamCancelFn = context.WithCancel(context.Background())
	streamCtxMu.Unlock()

	// Ensure the stream is cleaned up when the handler exits
	defer func() {
		log.Printf("Streamer %s disconnected from %s", user, r.RemoteAddr)
		streamActive.Store(false) // Mark stream as inactive
		streamCancelFn()          // Signal listeners to stop
		clearListeners()          // Close all listener channels
		resetStreamState()        // Prepare for a new stream
	}()

	buf := make([]byte, 1024)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			firstDataOnce.Do(func() {
				log.Println("First stream data received; unblocking listeners")
				close(firstData) // Signal listeners that data has started
			})
			broadcast(buf[:n])
		}
		if err != nil {
			log.Printf("Streamer read error for %s from %s: %v", user, r.RemoteAddr, err)
			break // Streamer disconnected or error
		}
	}
}

func listenHandler(w http.ResponseWriter, r *http.Request) {
	// Get the current stream context for this listener
	streamCtxMu.Lock()
	currentStreamCtx := streamCtx // Capture the current stream's context
	streamCtxMu.Unlock()

	// Wait for the current stream to start, or if no stream is active, continue.
	select {
	case <-firstData:
		// Stream has started, continue
	case <-r.Context().Done():
		// Client disconnected before stream started
		log.Printf("Listener from %s disconnected before stream started.", r.RemoteAddr)
		return
	case <-currentStreamCtx.Done():
		// Streamer disconnected before this listener received first data
		log.Printf("Listener from %s disconnected because streamer ended before first data.", r.RemoteAddr)
		http.Error(w, "No active stream", http.StatusServiceUnavailable)
		return
	}

	// If no stream is active when a listener connects, inform them.
	if !streamActive.Load() {
		http.Error(w, "No active stream", http.StatusServiceUnavailable)
		log.Printf("Listener from %s rejected: No active stream.", r.RemoteAddr)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive") // Keep the connection open

	ch := make(chan []byte, 100) // Buffer to prevent blocking broadcaster
	registerListener(ch)
	defer unregisterListener(ch) // Ensure listener is unregistered

	// Send the buffered recent audio data to the new listener first
	ringBufferMu.Lock()
	bufferedData := ringBuffer.Bytes()
	ringBufferMu.Unlock()

	if len(bufferedData) > 0 {
		if _, err := w.Write(bufferedData); err != nil {
			log.Printf("Error writing buffered data to listener from %s: %v", r.RemoteAddr, err)
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		log.Printf("Sent %d bytes of buffered data to new listener from %s", len(bufferedData), r.RemoteAddr)
	}

	// Loop to send subsequent live data
	for {
		select {
		case data := <-ch:
			if _, err := w.Write(data); err != nil {
				log.Printf("Error writing live data to listener from %s: %v", r.RemoteAddr, err)
				return // Client disconnected or error
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			log.Printf("Listener from %s disconnected.", r.RemoteAddr)
			return // Client disconnected
		case <-currentStreamCtx.Done():
			log.Printf("Listener from %s disconnected due to streamer ending.", r.RemoteAddr)
			return // Streamer disconnected, context cancelled
		}
	}
}

func broadcast(data []byte) {
	// Write to ring buffer
	ringBufferMu.Lock()
	if ringBuffer.Len()+len(data) > maxRingBufferSize {
		// If adding new data exceeds buffer size, make room by dropping oldest data.
		// A simple way is to reset the buffer and only keep the tail.
		// For a true ring buffer, you'd manage an offset. For simplicity, we'll
		// keep it simple here by trimming.
		temp := make([]byte, 0, maxRingBufferSize)
		// Copy only the part that fits and is newest
		copyLen := maxRingBufferSize - len(data)
		if copyLen < 0 { // If new data is larger than whole buffer
			copyLen = 0
		}
		if ringBuffer.Len() > copyLen {
			temp = append(temp, ringBuffer.Bytes()[ringBuffer.Len()-copyLen:]...)
		} else {
			temp = append(temp, ringBuffer.Bytes()...)
		}
		ringBuffer.Reset()
		ringBuffer.Write(temp)
	}
	ringBuffer.Write(data)
	ringBufferMu.Unlock()

	listenersMu.Lock()
	defer listenersMu.Unlock()
	for ch := range listeners {
		select {
		case ch <- data:
		default:
			// Drop if listener is slow, but log it.
			// This is expected if a client is very slow or has disconnected
			// but its goroutine hasn't fully exited yet.
			log.Printf("Dropped data for a slow listener.")
		}
	}
}

func registerListener(ch chan []byte) {
	listenersMu.Lock()
	listeners[ch] = struct{}{}
	listenersMu.Unlock()
	log.Printf("Registered new listener. Total listeners: %d", len(listeners))
}

func unregisterListener(ch chan []byte) {
	listenersMu.Lock()
	delete(listeners, ch)
	// Do NOT close(ch) here. It's either closed by clearListeners (streamer disconnects)
	// or will be garbage collected when the listener goroutine exits and no
	// other references to 'ch' remain. Closing here leads to "close of closed channel" panics.
	listenersMu.Unlock()
	log.Printf("Unregistered listener. Total listeners: %d", len(listeners))
}

// clearListeners closes all active listener channels.
func clearListeners() {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	for ch := range listeners {
		close(ch)             // Close the channel to signal end of stream
		delete(listeners, ch) // Remove from map
	}
	log.Println("All listener channels cleared due to streamer disconnection.")
}

func parseBasicAuth(r *http.Request) (username, password string, ok bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		return
	}

	payload, err := base64.StdEncoding.DecodeString(auth[len("Basic "):])
	if err != nil {
		return
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return
	}

	return pair[0], pair[1], true
}