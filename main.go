package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"log"
	"os"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/golang/freetype/truetype"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/golang/freetype"

	"github.com/karalabe/hid"
)

var testData = []byte{
	119, 97, 110, 103, 0, 0, 0, 0, 69, 69, 69, 69, 69, 69, 69, 69, 0, 11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 19, 8, 30, 23, 11, 39, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 25, 0, 17, 0, 4, 64, 32, 63, 0, 0, 0, 128, 7, 4, 4, 7, 64, 132, 135, 0, 0, 0, 0, 128, 128, 0, 128, 159, 149, 149, 0, 0, 0, 0, 0, 4, 36, 4, 36, 36, 36, 0, 0, 0, 0, 1, 1, 241, 145, 241, 129, 240, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 51, 0, 17, 0, 4, 0, 96, 63, 0, 0, 0, 0, 7, 4, 4, 7, 0, 196, 135, 0, 0, 0, 0, 128, 128, 0, 128, 159, 149, 149, 0, 0, 0, 0, 0, 4, 36, 4, 36, 36, 36, 0, 0, 0, 0, 1, 1, 241, 145, 241, 129, 240, 1, 0, 0, 0, 0, 0, 0, 0, 0}

// DisplayMode how message is displayed on screen
type DisplayMode int

const (
	// ModeScrollLeft scrolls from right to left
	ModeScrollLeft DisplayMode = iota
	// ModeScrollRight scrolls from left to right
	ModeScrollRight
	// ModeScrollUp gfx scrolls up
	ModeScrollUp
	// ModeScrollDown gfx scrolls down
	ModeScrollDown
	// ModeStillCenter gfx stays in middle
	ModeStillCenter
	// ModeAnimation quickly repeats frames
	ModeAnimation
	// ModeDropDown drop-down
	ModeDropDown
	// ModeCurtain opening-closing curtains
	ModeCurtain
	// ModeLaser laser-engraving
	ModeLaser
)

// Message one frame
type Message struct {
	Speed   int
	Mode    DisplayMode
	Border  bool
	Blink   bool
	Columns [][11]byte
}

// ErrBadImageSize if image size is wrong (e.g. not 11 pixels high)
var ErrBadImageSize = errors.New("Bad image size")

// SetImage add picture data from image
func (m *Message) SetImage(img image.Image) error {
	bounds := img.Bounds()
	if bounds.Dy() != 11 {
		return ErrBadImageSize
	}
	imgWidth := bounds.Dx()
	nrCells := imgWidth / 8
	if imgWidth%8 != 0 {
		nrCells++
	}
	m.Columns = make([][11]byte, nrCells)
	for i := 0; i < imgWidth; i += 8 {
		for j := 0; j < 11; j++ {
			for b := 0; b < 8 && i+b < imgWidth; b++ {
				pixelValue := color.GrayModel.Convert(img.At(i+b, j)).(color.Gray).Y
				if pixelValue > 0x7f {
					m.Columns[i/8][j] |= 1 << uint(7-b)
				}
			}
		}
	}
	return nil
}

// Packet to send to device
type Packet struct {
	Magic     [6]byte
	Timestamp time.Time
	Messages  []Message
}

type packetHeader struct {
	Magic         [6]byte
	BlinkAttr     byte
	BorderAttr    byte
	SpeedAndMode  [8]byte
	MessageLength [8]uint16
	_             [6]byte
	Tm            struct {
		Yr byte
		Mo byte
		Dy byte
		Hr byte
		Mi byte
		Sc byte
	}
	_ [20]byte
}

// UnmarshalBinary parse binary blob
func (p *Packet) UnmarshalBinary(data []byte) error {
	buf := bytes.NewBuffer(data)
	var ph packetHeader
	if err := binary.Read(buf, binary.BigEndian, &ph); err != nil {
		return err
	}
	p.Magic = [6]byte(ph.Magic)
	var err error
	p.Timestamp, err = time.Parse("06-01-02 15:04:05", fmt.Sprintf("%02d-%02d-%02d %02d:%02d:%02d", ph.Tm.Yr, ph.Tm.Mo, ph.Tm.Dy, ph.Tm.Hr, ph.Tm.Mi, ph.Tm.Sc))
	if err != nil {
		return err
	}
	p.Messages = []Message{}
	for idx, msgLength := range ph.MessageLength {
		// log.Printf("Message[%d]: %d 8-bit columns", idx, msgLength)
		if msgLength == 0 {
			continue
		}
		msg := Message{
			Speed:   int(ph.SpeedAndMode[idx]&0xf0) >> 4,
			Mode:    DisplayMode(ph.SpeedAndMode[idx] & 0x0f),
			Blink:   ph.BlinkAttr&(1<<uint(idx)) != 0,
			Border:  ph.BorderAttr&(1<<uint(idx)) != 0,
			Columns: make([][11]byte, msgLength),
		}
		if err := binary.Read(buf, binary.BigEndian, &msg.Columns); err != nil {
			return err
		}

		p.Messages = append(p.Messages, msg)
	}
	return nil
}

// MarshalBinary make a binary blob
func (p Packet) MarshalBinary() (data []byte, err error) {
	d := new(bytes.Buffer)
	ph := packetHeader{Magic: p.Magic}
	ph.Tm.Yr = byte(p.Timestamp.Year() % 100)
	ph.Tm.Mo = byte(p.Timestamp.Month())
	ph.Tm.Dy = byte(p.Timestamp.Day())
	ph.Tm.Hr = byte(p.Timestamp.Hour())
	ph.Tm.Mi = byte(p.Timestamp.Minute())
	ph.Tm.Sc = byte(p.Timestamp.Second())
	messageBytes := []byte{}

	for idx, msg := range p.Messages {
		ph.SpeedAndMode[idx] = byte(msg.Speed<<4) + byte(msg.Mode)
		ph.MessageLength[idx] = uint16(len(msg.Columns))
		if msg.Blink {
			ph.BlinkAttr |= (1 << uint(idx))
		}
		if msg.Border {
			ph.BorderAttr |= (1 << uint(idx))
		}
		for _, fb := range msg.Columns {
			messageBytes = append(messageBytes, fb[:]...)
		}
	}

	if err := binary.Write(d, binary.BigEndian, ph); err != nil {
		return nil, err
	}
	if n, err := d.Write(messageBytes); err != nil {
		log.Printf("Could not add message[%d]: %s", n, err)
	}
	if l := d.Len() % 64; l != 0 {
		d.Write(make([]byte, 64-l))
	}
	return d.Bytes(), nil
}

// NewPacket creates new empty Packet
func NewPacket() (r *Packet) {
	r = &Packet{Magic: [6]byte{'w', 'a', 'n', 'g'}}
	r.Timestamp = time.Now()
	r.Messages = []Message{}
	return r
}

// TextGenerator creates an image from text with specified font parameters
type TextGenerator struct {
	FontFile string
	Face     font.Face
	Hinting  font.Hinting
	FontSize float64
	DPI      float64
	Base     int
}

// DrawString renders text string and returns image
func (d *TextGenerator) DrawString(s string) (img image.Image, err error) {
	if d.Face == nil {
		fontBytes, err := ioutil.ReadFile(d.FontFile)
		if err != nil {
			return nil, err
		}

		ftFont, err := freetype.ParseFont(fontBytes)
		if err != nil {
			return nil, err
		}
		d.Face = truetype.NewFace(ftFont, &truetype.Options{
			Size:    d.FontSize,
			DPI:     d.DPI,
			Hinting: d.Hinting,
		})
	}
	adv := font.MeasureString(d.Face, s)
	fontDrawer := &font.Drawer{
		Dst:  image.NewGray(image.Rect(0, 0, adv.Ceil(), 11)),
		Src:  image.White,
		Face: d.Face,
	}
	fontDrawer.Dot = fixed.P(0, d.Base)
	fontDrawer.DrawString(s)

	return fontDrawer.Dst, nil
}

func main() {
	usbIDFlag := flag.String("devid", "0416:5020", "USB device ID")
	deviceIndex := flag.Int("devnr", 0, "device index (default 0)")
	fontFileFlag := flag.String("font", "k8x12/k8x12.ttf", "TTF font file for text (cf: http://littlelimit.net/k8x12.htm)")
	fontSizeFlag := flag.Float64("fs", 12, "Font size")
	fontDPIFlag := flag.Float64("dpi", 72, "Font DPI")
	fontBaseFlag := flag.Int("base", 10, "Base pixel")
	testFlag := flag.Bool("test", false, "use test data")
	modeFlag := flag.String("mode", "left", "mode [left/right/up/down/center/anim/drop/curtain/laser] (can be changed between messages)")
	speedFlag := flag.Int("speed", 5, "speed (can be changed between messages)")
	blinkFlag := flag.Bool("blink", false, "blink (can be changed between messages with -/+blink)")
	borderFlag := flag.Bool("border", false, "border (can be changed between messages with -/+border)")
	gfxFlag := flag.String("gfx", "", "Load image from file instead of message (repeatable)")
	hintingFlag := flag.String("hinting", "full", "TTF hinting (full / none)")
	flag.Parse()
	var usbIDVendor, usbIDDevice uint16
	fmt.Sscanf(*usbIDFlag, "%04x:%04x", &usbIDVendor, &usbIDDevice)
	di, err := hid.Enumerate(usbIDVendor, usbIDDevice)
	if len(di) < 1 {
		log.Fatalf("Could not find any devices")
	}
	dev := di[*deviceIndex]
	devOpen, err := di[*deviceIndex].Open()
	if err != nil {
		log.Fatalf("Could not open device %#v: %s", di[0], err)
	}
	defer devOpen.Close()
	log.Printf("Opened %s / %s @ %s", dev.Manufacturer, dev.Product, dev.Path)

	var p *Packet
	if *testFlag {
		data := testData
		p = &Packet{}
		if err := p.UnmarshalBinary(data); err != nil {
			log.Fatalf("Could not unmarshal: %s", err)
		}
		p.Messages[0].Speed = 6
		msg := Message{Speed: 0, Mode: ModeAnimation, Border: true}
		img := image.NewGray(image.Rectangle{Max: image.Point{X: 25, Y: 11}})
		img.SetGray(10, 5, color.Gray{255})
		if err := msg.SetImage(img); err != nil {
			log.Printf("Cannot set image: %s", err)
		} else {
			p.Messages = append(p.Messages, msg)
		}
	} else {
		p = NewPacket()
		skipNextArg := false
		args := flag.Args()
		ti := &TextGenerator{FontFile: *fontFileFlag, FontSize: *fontSizeFlag, DPI: *fontDPIFlag, Base: *fontBaseFlag}
		if *hintingFlag == "full" {
			ti.Hinting = font.HintingFull
		}
		for idx, arg := range args {
			if skipNextArg {
				skipNextArg = false
				continue
			}
			switch arg {
			case "-mode", "-speed", "-gfx":
				flag.Set(arg[1:], args[idx+1])
				skipNextArg = true
				continue
			case "-blink":
				*blinkFlag = true
				continue
			case "+blink":
				*blinkFlag = false
				continue
			case "-border":
				*borderFlag = true
				continue
			case "+border":
				*borderFlag = false
				continue
			}
			var mode DisplayMode
			switch *modeFlag {
			case "left":
				mode = ModeScrollLeft
			case "right":
				mode = ModeScrollRight
			case "up":
				mode = ModeScrollUp
			case "down":
				mode = ModeScrollDown
			case "center":
				mode = ModeStillCenter
			case "anim":
				mode = ModeAnimation
			case "drop":
				mode = ModeDropDown
			case "curtain":
				mode = ModeCurtain
			case "laser":
				mode = ModeLaser
			}
			msg := Message{Mode: mode, Speed: *speedFlag, Blink: *blinkFlag, Border: *borderFlag}

			if *gfxFlag != "" {
				infile, err := os.Open(*gfxFlag)
				if err != nil {
					log.Fatalf("Cannot open file %#v: %s", *gfxFlag, err)
				}
				img, _, err := image.Decode(infile)
				if err != nil {
					log.Fatalf("Cannot load image from %#v: %s", *gfxFlag, err)
				}
				if err := msg.SetImage(img); err != nil {
					log.Fatalf("Cannot set image %#v as image: %s", *gfxFlag, err)
				}
				flag.Set("gfx", "")
			} else {
				textImage, err := ti.DrawString(arg)
				if err != nil {
					log.Fatal("Cannot draw text: ", err)
				}
				msg.SetImage(textImage)
			}

			p.Messages = append(p.Messages, msg)
		}
	}

	buf, err := p.MarshalBinary()
	if err != nil {
		log.Printf("Cannot encode packet: %s", err)
	}
	fmt.Printf("Marshalled data:\n%s", hex.Dump(buf))
	if l := len(buf); l > 8192 {
		log.Fatalf("Too long buffer (%d), max is 8192", l)
	}

	_, err = devOpen.Write(buf)
	if err != nil {
		log.Fatalf("Could not write to device %#v: %s", di[0], err)
	}
}
