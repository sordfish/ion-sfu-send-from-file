package main

import (
	"io"
	"os"
	"time"

	"fmt"
	"net/http"

	ilog "github.com/pion/ion-log"
	sdk "github.com/pion/ion-sdk-go"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/h264reader"
)

var (
	log = ilog.NewLoggerWithFields(ilog.DebugLevel, "", nil)
)

func healthz(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "OK\n")
}

const (
	address           = "localhost:50051"
	audioFileName     = "output.ogg"
	videoFileName     = "output.h264"
	oggPageDuration   = time.Millisecond * 20
	h264FrameDuration = time.Millisecond * 33
)

func main() {

	// Assert that we have an audio or video file
	_, err := os.Stat(videoFileName)
	haveVideoFile := !os.IsNotExist(err)

	_, err = os.Stat(audioFileName)
	haveAudioFile := !os.IsNotExist(err)

	if !haveAudioFile && !haveVideoFile {
		panic("Could not find `" + audioFileName + "` or `" + videoFileName + "`")
	}

	env_addr := os.Getenv("ISGS_ADDR")
	env_session := os.Getenv("ISGS_SESSION")
	env_videoSrc := os.Getenv("ISGS_VIDEO_SRC")
	env_audioSrc := os.Getenv("ISGS_AUDIO_SRC")
	env_turnAddr := os.Getenv("ISGS_TURN_ADDR")
	env_turnUser := os.Getenv("ISGS_TURN_USER")
	env_turnPass := os.Getenv("ISGS_TURN_PASS")

	log.Infof("This is the env addr: %s", env_addr)
	log.Infof("This is the env session: %s", env_session)
	log.Infof("This is the env videosrc: %s", env_videoSrc)
	log.Infof("This is the env audiosrc: %s", env_audioSrc)
	log.Infof("This is the env turnaddr: %s", env_turnAddr)
	log.Infof("This is the env turnaddr: %s", env_turnUser)
	log.Infof("This is the env turnaddr: %s", env_turnPass)

	servicename, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	var webrtcCfg webrtc.Configuration

	if len(env_turnAddr) > 0 {

		webrtcCfg = webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs:       []string{"turn:" + env_turnAddr},
					Username:   env_turnUser,
					Credential: env_turnPass,
				},
			},
		}

	} else {

		webrtcCfg = webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				webrtc.ICEServer{},
			},
		}

	}

	config := sdk.RTCConfig{
		WebRTC: sdk.WebRTCTransportConfig{
			VideoMime:     "video/h264",
			Configuration: webrtcCfg,
		},
	}

	mediaEngine := webrtc.MediaEngine{}
	mediaEngine.RegisterDefaultCodecs()

	connector := sdk.NewConnector(env_addr)
	rtc := sdk.NewRTC(connector, config)

	//videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/h264", ClockRate: 90000, Channels: 0, SDPFmtpLine: "packetization-mode=1;profile-level-id=42e01f", RTCPFeedback: nil}, "video", servicename)
	videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/h264", ClockRate: 90000, Channels: 0, RTCPFeedback: nil}, "video", servicename)
	if err != nil {
		panic(err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", servicename)
	if err != nil {
		panic(err)
	}

	// client join a session
	err = rtc.Join(env_session, sdk.RandomKey(4))

	if err != nil {
		log.Errorf("join err=%v", err)
		panic(err)
	}
	_, _ = rtc.Publish(videoTrack, audioTrack)

	// Start pushing buffers on these tracks
	go func() {
		// Open a H264 file and start reading using our IVFReader
		file, h264Err := os.Open(videoFileName)
		if h264Err != nil {
			panic(h264Err)
		}

		h264, h264Err := h264reader.NewReader(file)
		if h264Err != nil {
			panic(h264Err)
		}

		// Wait for connection established
		// <-iceConnectedCtx.Done()

		// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
		// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
		//
		// It is important to use a time.Ticker instead of time.Sleep because
		// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
		// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
		ticker := time.NewTicker(h264FrameDuration)
		for ; true; <-ticker.C {
			nal, h264Err := h264.NextNAL()
			if h264Err == io.EOF {
				fmt.Printf("All video frames parsed and sent")
				os.Exit(0)
			}
			if h264Err != nil {
				panic(h264Err)
			}

			if h264Err = videoTrack.WriteSample(media.Sample{Data: nal.Data, Duration: time.Second}); h264Err != nil {
				panic(h264Err)
			}
		}
	}()

	http.HandleFunc("/healthz", healthz)
	http.ListenAndServe(":8090", nil)

	select {}

}
