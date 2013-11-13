// Copyright 2013 Örjan Persson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"time"

	"code.google.com/p/portaudio-go/portaudio"
	"github.com/op/go-libspotify"
)

var (
	appKeyPath = flag.String("key", "spotify_appkey.key", "path to app.key")
	username   = flag.String("username", "o.p", "spotify username")
	password   = flag.String("password", "", "spotify password")
	debug      = flag.Bool("debug", false, "debug output")
)

type audio struct {
	format *spotify.AudioFormat
	frames []byte
}

type audio2 struct {
	format *spotify.AudioFormat
	frames []int16
}

type portAudio struct {
	incoming chan *audio
	outgoing chan *audio2
	freelist chan []int16
}

func newPortAudio() *portAudio {
	return &portAudio{
		incoming: make(chan *audio, 8),
		outgoing: make(chan *audio2, 8),
		freelist: make(chan []int16, 12),
	}
}

func (pa *portAudio) WriteAudio(format *spotify.AudioFormat, frames []byte) int {
	audio := &audio{format, frames}
	println("audio", len(frames), len(frames)/2)

	if len(frames) == 0 {
		println("no frames")
		return 0
	}

	select {
	case pa.incoming <- audio:
		println("return", len(frames))
		return len(frames)
	default:
		println("buffer full")
		return 0
	}
}

func (pa *portAudio) processor() {
	var f []int16
	for in := range pa.incoming {
		select {
		case f = <-pa.freelist:
		default:
			f = make([]int16, 2048)
		}
		j := 0
		for i := 0; i < len(in.frames); i += 2 {
			f[j] = int16(in.frames[i]) | int16(in.frames[i+1])<<8
			j++
		}
		pa.outgoing <- &audio2{in.format, f}
	}
}

func (pa *portAudio) player() {
	var out []int16

	stream, err := portaudio.OpenDefaultStream(
		0,
		2,     // audio.format.Channels,
		44100, // float64(audio.format.SampleRate),
		2048,  // len(out),
		&out,
	)
	if err != nil {
		panic(err)
	}
	defer stream.Close()

	stream.Start()
	defer stream.Stop()

	for audio := range pa.outgoing {
		if len(audio.frames) != 4096/2 {
			panic("unexpected")
		}

		out = audio.frames
		stream.Write()

		select {
		case pa.freelist <- audio.frames:
		default:
		}
	}
}

func main() {
	flag.Parse()
	prog := path.Base(os.Args[0])

	portaudio.Initialize()
	defer portaudio.Terminate()

	if flag.NArg() != 1 {
		log.Fatal("Expects exactly one argument.")
	}
	uri := flag.Arg(0)

	appKey, err := ioutil.ReadFile(*appKeyPath)
	if err != nil {
		log.Fatal(err)
	}

	pa := newPortAudio()
	go pa.player()
	go pa.processor()

	session, err := spotify.NewSession(&spotify.Config{
		ApplicationKey:   appKey,
		ApplicationName:  prog,
		CacheLocation:    "tmp",
		SettingsLocation: "tmp",
		AudioConsumer:    pa,
	})
	if err != nil {
		log.Fatal(err)
	}

	credentials := spotify.Credentials{
		Username: *username,
		Password: *password,
	}
	if err = session.Login(credentials, false); err != nil {
		log.Fatal(err)
	}

	select {
	case err := <-session.LoginUpdates():
		if err != nil {
			log.Fatal(err)
		}
	}

	link, err := session.ParseLink(uri)
	if err != nil {
		log.Fatal(err)
	}
	track, err := link.Track()
	if err != nil {
		log.Fatal(err)
	}

	track.Wait()
	player := session.Player()
	if err := player.Load(track); err != nil {
		fmt.Println("%#v", err)
		log.Fatal(err)
	}

	player.Play()

	for {
		time.Sleep(100 * time.Millisecond)
	}
}
