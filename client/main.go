package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	_ "image/png"
	"net"
	"os"
	"sync"

	"github.com/gen2brain/malgo"
	"gocv.io/x/gocv"
)

type destination struct {
	ip   string
	port string
}

func compressVideo(data []byte) []byte {
	imgMat, err := gocv.NewMatFromBytes(480, 640, gocv.MatTypeCV8UC3, data)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	img, err := imgMat.ToImage()
	if err != nil {
		fmt.Println("Error converting matrix to image")
		fmt.Println(err)
		os.Exit(-1)
	}
	var buf bytes.Buffer
	options := &jpeg.Options{Quality: 80}
	jpeg.Encode(&buf, img, options)
	if buf.Len() > 50000 {
		options = &jpeg.Options{Quality: 50}
		jpeg.Encode(&buf, img, options)
	}
	if buf.Len() > 50000 {
		options = &jpeg.Options{Quality: 20}
		jpeg.Encode(&buf, img, options)
	}
	if buf.Len() > 50000 {
		options = &jpeg.Options{Quality: 1}
		jpeg.Encode(&buf, img, options)
	}
	return buf.Bytes()
}

/*
*	Sends the data to dst
 */
func sendData(message []byte, dst *destination) {
	conn, err := net.Dial("udp", dst.ip+":"+dst.port)
	if err != nil {
		fmt.Println("[-] Error oppening UDP socket to IP " + dst.ip + " on port " + dst.port)
		fmt.Println("[-] Failed to send message")
		os.Exit(-1)
	}

	_, err = conn.Write(message)
	if err != nil {
		fmt.Println(len(message))
		fmt.Println(err)
	}
	defer conn.Close()
}

func handleMessage(msg string) {
	if msg == "Hello world" {
		fmt.Println("Sending data")
	}
}

func handleSendingVideo() {
	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		fmt.Printf("Error opening webcam: %v\n", err)
		return
	}
	defer webcam.Close()

	// Create a Mat to hold the webcam frames
	img := gocv.NewMat()
	defer img.Close()

	dst1 := destination{"127.0.0.1", "9000"}
	for {
		if ok := webcam.Read(&img); !ok {
			fmt.Println("Cannot read device")
			return
		}
		if img.Empty() {
			continue
		}
		packet := prepVideoPacket(img.ToBytes())
		if packet != nil {
			sendData(packet, &dst1)
		}

	}
}

func handleSendingAudio() {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {

	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Duplex)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 1
	deviceConfig.SampleRate = 44100
	deviceConfig.Alsa.NoMMap = 1

	var capturedSampleCount uint32
	pCapturedSamples := make([]byte, 0)

	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {
		sampleCount := framecount * deviceConfig.Capture.Channels * sizeInBytes
		newCapturedSampleCount := capturedSampleCount + sampleCount
		pCapturedSamples = append(pCapturedSamples, pSample...)
		capturedSampleCount = newCapturedSampleCount

	}

	fmt.Println("Recording...")
	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, captureCallbacks)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = device.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer device.Uninit()

	dst1 := destination{"127.0.0.1", "9000"}
	for {
		packet := prepSoundPacket(&pCapturedSamples)
		if packet != nil {
			sendData(packet, &dst1)
		}
	}

}

func main() {
	var wg sync.WaitGroup
	go handleSendingAudio()
	go handleSendingVideo()
	wg.Add(1)
	wg.Wait()
}

func prepSoundPacket(data *[]byte) []byte {
	if len(*data) > 44100 {
		toReturn := (*data)[:44100]
		*data = (*data)[44100:]
		return createPacket(1, toReturn)
	} else {
		return nil
	}
}

func prepVideoPacket(data []byte) []byte {
	if len(data) > 0 {
		data = compressVideo(data)
		return createPacket(2, data)
	} else {
		return nil
	}
}

func createPacket(packetID int32, data []byte) []byte {
	toReturn := make([]byte, 0)
	packetIDAsBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(packetIDAsBytes, uint32(packetID))
	toReturn = append(toReturn, packetIDAsBytes...)
	dataLengthAsBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(dataLengthAsBytes, uint32(len(data)))
	toReturn = append(toReturn, dataLengthAsBytes...)
	toReturn = append(toReturn, data...)
	return toReturn
}
