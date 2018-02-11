package main

import (
	"sync"

	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/rtmp"
)

type activeStream struct {
	stream       stream
	streamCodecs []av.CodecData
}

type streamServer struct {
	server        *rtmp.Server
	activeStreams []activeStream
	mutex         *sync.Mutex
}

func newRTMPServer(e *env) *streamServer {
	return &streamServer{
		&rtmp.Server{},
		[]activeStream{},
		&sync.Mutex{},
	}
}

func (s *streamServer) handlePublish(conn *rtmp.Conn) {
	s.mutex.Lock()
	_, exists := s.activeStreams[conn.URL.Path]

}
