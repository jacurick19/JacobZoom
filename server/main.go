package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	"net"
	"os"
	"sync"

	"github.com/gen2brain/malgo"
	"github.com/hajimehoshi/ebiten"
)

var screenPixels []byte

/*
*	Recieves data, returning a string
 */
func recvData(conn *net.UDPConn, messages chan []byte) {
	for {
		//
		recievedBytes := make([]byte, 70000)
		_, _, err := conn.ReadFrom(recievedBytes)
		if err != nil {
			fmt.Println("Error recieving data")
			fmt.Println(err)
		}
		messages <- recievedBytes
	}
}

func initServer() {
	recievedMessages := make(chan []byte)
	udpAddr, err := net.ResolveUDPAddr("udp", os.Args[1])
	if err != nil {
		fmt.Println("Fatal Error")
		fmt.Println(err)
		os.Exit(-1)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Fatal error")
		fmt.Println(err)
		os.Exit(-1)
	}
	go recvData(conn, recievedMessages)
	for {
		msg := <-recievedMessages
		go handleMessage(msg)
	}
}

/*
*	Parses a message of the form
*		packetID:INT
*		length:INT
*		data:BYTES
*	Returns packetID, length, data
*
 */
func parsePacket(packet []byte) (uint32, uint32, []byte) {
	packetIDBytes := make([]byte, 0)
	packetIDBytes = append(packetIDBytes, packet[0], packet[1], packet[2], packet[3])
	lengthBytes := make([]byte, 0)
	lengthBytes = append(lengthBytes, packet[4], packet[5], packet[6], packet[7])
	dataBytes := packet[8 : binary.BigEndian.Uint32(lengthBytes)+8]
	return binary.BigEndian.Uint32(packetIDBytes), binary.BigEndian.Uint32(lengthBytes), dataBytes
}

func handleAudioMessage(length uint32, data []byte) {

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Duplex)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 1
	deviceConfig.SampleRate = 44100
	deviceConfig.Alsa.NoMMap = 1
	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))
	var playbackSampleCount uint32
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
	onSendFrames := func(pSample, nil []byte, framecount uint32) {
		samplesToRead := framecount * deviceConfig.Playback.Channels * sizeInBytes
		if samplesToRead > length-playbackSampleCount {
			samplesToRead = length - playbackSampleCount
		}

		copy(pSample, data[playbackSampleCount:playbackSampleCount+samplesToRead])

		playbackSampleCount += samplesToRead
		if playbackSampleCount >= length {
			return
		}
	}

	playbackCallbacks := malgo.DeviceCallbacks{
		Data: onSendFrames,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, playbackCallbacks)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = device.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Scanln()

	device.Uninit()
}

func decompressVideo(data []byte) []byte {
	reader := bytes.NewReader(data)
	img, _, err := image.Decode(reader)
	if err != nil {
		fmt.Println(data)
		fmt.Println(err)
		os.Exit(0)
	}
	rgba := image.NewRGBA(img.Bounds())

	// Fill the RGBA image with the JPEG image data
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba.Pix
}

func handleVideoMessage(length uint32, data []byte) {
	img := decompressVideo(data)
	screenPixels = img
}

func update(screen *ebiten.Image) error {
	// Display the image
	if len(screenPixels) == 1228800 {
		screen.ReplacePixels(screenPixels)
	}

	return nil
}

func handleMessage(msg []byte) {
	packetID, length, data := parsePacket(msg)
	if packetID == 1 {
		handleAudioMessage(length, data)
	} else if packetID == 2 {
		//	time.Sleep(time.Millisecond * 200)
		handleVideoMessage(length, data)
	} else {
		fmt.Println(msg[:10])
	}
}

func main() {
	var wg sync.WaitGroup
	go initServer()
	if err := ebiten.Run(update, 640, 480, 1, "JacobZoom"); err != nil {
		fmt.Println(err)
	}
	wg.Add(1)
	wg.Wait()
}
