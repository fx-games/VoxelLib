package tpack

import (
	"github.com/memmaker/voxellib/tpack/png_chunks"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path"
	"strings"
)

func LoadTrimmedAtlas(filename string) (image.Image, TrimmedAtlasMetadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	return DecodeTrimmedAtlas(file)
}

func DecodeTrimmedAtlas(inReader io.ReadSeeker) (image.Image, TrimmedAtlasMetadata, error) {
	img, err := png.Decode(inReader)
	if err != nil {
		return nil, nil, err
	}
	inReader.Seek(0, io.SeekStart)
	metadata := png_chunks.ReadChunkFromPNG(inReader, "atLS")
	decodedMeta := NewTrimmedAtlasMetadataFromBytes(metadata)
	return img, decodedMeta, nil
}

func WritePackedAtlasToFiles(image *image.RGBA, meta AtlasMetadata, dstPathAndBaseName string) error {
	imagePath := dstPathAndBaseName + ".png"
	atlasPath := dstPathAndBaseName + ".txt"

	imageErr := writeImageToPNG(image, imagePath)
	if imageErr != nil {
		return imageErr
	}

	atlasErr := os.WriteFile(atlasPath, []byte(meta.String()), 0666)
	if atlasErr != nil {
		return atlasErr
	}
	return nil
}

func PackImagesIntoAtlas(images []NamedImage) (*image.RGBA, AtlasMetadata) {
	var rectList []RectangleWithIndex
	for index, sprite := range images {
		bounds := sprite.Bounds()
		rect := RectangleWithIndex{
			Rectangle: Rectangle{
				Width:  bounds.Dx(),
				Height: bounds.Dy(),
			},
			Index: index,
		}

		rectList = append(rectList, rect)
	}
	targetAtlasWidth := 2048
	targetAtlasHeight := 8192 * 2
	binPack := NewPacker(targetAtlasWidth, targetAtlasHeight)
	binPack.InsertAll(rectList, RULE_BEST_AREA_FIT)
	binPack.ShrinkBin(false)
	placedRects := binPack.GetUsedRectangles()
	neededWidth := makeEven(binPack.GetWidth())
	neededHeight := makeEven(binPack.GetHeight())
	dst := image.NewRGBA(image.Rect(0, 0, neededWidth, neededHeight))

	atlasEntries := make(AtlasMetadata, 0)
	for _, rect := range placedRects {
		sprite := images[rect.Index]
		bounds := sprite.Bounds()
		nrgba := AsRGBA(sprite)
		spriteName := path.Base(sprite.Name)
		if strings.HasSuffix(strings.ToLower(spriteName), ".png") {
			spriteName = spriteName[:len(spriteName)-4]
		}
		atlasEntries = append(atlasEntries, AtlasEntry{
			SpriteName: LPString8(spriteName),
			X:          uint16(rect.X),
			Y:          uint16(rect.Y),
			Width:      uint16(rect.Width),
			Height:     uint16(rect.Height),
		})
		draw.Draw(dst, rect.ToImageRect(), nrgba, bounds.Min, draw.Src)
	}
	return dst, atlasEntries
}

func PackImagesIntoAtlasTrimmed(images []NamedImage) (*image.RGBA, TrimmedAtlasMetadata) {
	var rectList []RectangleWithIndex
	var trimRects []image.Rectangle
	for index, sprite := range images {
		trimRect := getTrimRect(sprite)
		rect := RectangleWithIndex{
			Rectangle: Rectangle{
				Width:  trimRect.Bounds().Dx(),
				Height: trimRect.Bounds().Dy(),
			},
			Index: index,
		}
		trimRects = append(trimRects, trimRect)
		rectList = append(rectList, rect)
	}
	targetAtlasWidth := 2048
	targetAtlasHeight := 8192 * 2
	binPack := NewPacker(targetAtlasWidth, targetAtlasHeight)
	binPack.InsertAll(rectList, RULE_BEST_AREA_FIT)
	binPack.ShrinkBin(false)
	placedRects := binPack.GetUsedRectangles()
	neededWidth := makeEven(binPack.GetWidth())
	neededHeight := makeEven(binPack.GetHeight())
	dst := image.NewRGBA(image.Rect(0, 0, neededWidth, neededHeight))

	atlasEntries := make(TrimmedAtlasMetadata, 0)
	for _, rect := range placedRects {
		sprite := images[rect.Index]
		bounds := sprite.Bounds()
		nrgba := AsRGBA(sprite)
		spriteName := path.Base(sprite.Name)
		if strings.HasSuffix(strings.ToLower(spriteName), ".png") {
			spriteName = spriteName[:len(spriteName)-4]
		}

		trimRect := trimRects[rect.Index]
		atlasEntries = append(atlasEntries, TrimmedAtlasEntry{
			SpriteName:     LPString8(spriteName),
			X:              uint16(rect.X),
			Y:              uint16(rect.Y),
			SourceWidth:    uint16(bounds.Dx()),
			SourceHeight:   uint16(bounds.Dy()),
			TrimRectWidth:  uint16(rect.Width),
			TrimRectHeight: uint16(rect.Height),
			TrimRectX:      uint16(trimRect.Min.X),
			TrimRectY:      uint16(trimRect.Min.Y),
			OffsetX:        clampInt16(sprite.OffsetX),
			OffsetY:        clampInt16(sprite.OffsetY),
		})
		draw.Draw(dst, rect.ToImageRect(), nrgba, trimRect.Min, draw.Src)
	}
	return dst, atlasEntries
}
