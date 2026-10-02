//go:build darwin && cgo

package photoencode

/*
#cgo LDFLAGS: -framework ImageIO -framework CoreGraphics -framework CoreFoundation
#include <ImageIO/ImageIO.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>
typedef struct {unsigned char *pixels,*icc; size_t width,height,iccLen; int orientation,gray; char date[64];const char *error;} DGSImage;
static DGSImage dgsFail(DGSImage r,const char *error){free(r.icc);free(r.pixels);r.icc=NULL;r.pixels=NULL;r.error=error;return r;}
// dgsDecode renders the first and only image in its own colour space: RGB into
// premultiplied RGBA, monochrome into 8-bit gray over the background level.
static DGSImage dgsDecode(const void *bytes,size_t length,size_t maxPixels,int preserveGPS,int allowDeep,double background){
 DGSImage r={0};r.orientation=1;
 CFDataRef data=CFDataCreate(NULL,bytes,length);CGImageSourceRef source=CGImageSourceCreateWithData(data,NULL);CFRelease(data);
 if(!source)return dgsFail(r,"cannot open image");
 if(CGImageSourceGetCount(source)!=1){CFRelease(source);return dgsFail(r,"multi-image files are not supported");}
 CFDictionaryRef props=CGImageSourceCopyPropertiesAtIndex(source,0,NULL);
 if(!props){CFRelease(source);return dgsFail(r,"missing image properties");}
 CFNumberRef width=CFDictionaryGetValue(props,kCGImagePropertyPixelWidth),height=CFDictionaryGetValue(props,kCGImagePropertyPixelHeight);
 int w=0,h=0;if(width)CFNumberGetValue(width,kCFNumberIntType,&w);if(height)CFNumberGetValue(height,kCFNumberIntType,&h);
 if(w<=0||h<=0||(size_t)w>maxPixels/(size_t)h){CFRelease(props);CFRelease(source);return dgsFail(r,"image exceeds configured pixel limit");}
 CFNumberRef depth=CFDictionaryGetValue(props,kCGImagePropertyDepth);int bits=0;if(depth)CFNumberGetValue(depth,kCFNumberIntType,&bits);
 if(bits>8&&!allowDeep){CFRelease(props);CFRelease(source);return dgsFail(r,"high-bit-depth/HDR is not supported without colour conversion");}
 CFNumberRef orientation=CFDictionaryGetValue(props,kCGImagePropertyOrientation);if(orientation)CFNumberGetValue(orientation,kCFNumberIntType,&r.orientation);
 CFDictionaryRef gps=CFDictionaryGetValue(props,kCGImagePropertyGPSDictionary);
 if(preserveGPS&&gps){CFRelease(props);CFRelease(source);return dgsFail(r,"GPS preservation is not supported for this format yet; disable Preserve GPS");}
 CFDictionaryRef exif=CFDictionaryGetValue(props,kCGImagePropertyExifDictionary);if(exif){CFStringRef date=CFDictionaryGetValue(exif,kCGImagePropertyExifDateTimeOriginal);if(date)CFStringGetCString(date,r.date,sizeof(r.date),kCFStringEncodingUTF8);}
 CGImageRef image=CGImageSourceCreateImageAtIndex(source,0,NULL);CFRelease(props);CFRelease(source);
 if(!image)return dgsFail(r,"decode failed");
 CGColorSpaceRef space=CGImageGetColorSpace(image);CGColorSpaceModel model=space?CGColorSpaceGetModel(space):kCGColorSpaceModelUnknown;
 if((model!=kCGColorSpaceModelRGB&&model!=kCGColorSpaceModelMonochrome)||(CGImageGetBitmapInfo(image)&kCGBitmapFloatComponents)||(CGImageGetBitsPerComponent(image)>8&&!allowDeep)){CGImageRelease(image);return dgsFail(r,"colour representation is unsupported");}
 r.gray=model==kCGColorSpaceModelMonochrome;
 // An untagged image has no profile to carry, as with untagged JPEG and PNG.
 CFDataRef profile=CGColorSpaceCopyICCData(space);
 if(profile){
  r.iccLen=CFDataGetLength(profile);if(r.iccLen<128||r.iccLen>4*1024*1024){CFRelease(profile);CGImageRelease(image);return dgsFail(r,"ICC profile size is invalid");}
  r.icc=malloc(r.iccLen);if(!r.icc){CFRelease(profile);CGImageRelease(image);return dgsFail(r,"allocation failed");}memcpy(r.icc,CFDataGetBytePtr(profile),r.iccLen);CFRelease(profile);
 }
 r.width=CGImageGetWidth(image);r.height=CGImageGetHeight(image);
 if(r.width==0||r.height==0||r.width>maxPixels/r.height){CGImageRelease(image);return dgsFail(r,"decoded dimensions exceed limit");}
 size_t channels=r.gray?1:4;
 r.pixels=calloc(r.width*r.height,channels);
 CGContextRef context=r.pixels?CGBitmapContextCreate(r.pixels,r.width,r.height,8,r.width*channels,space,r.gray?kCGImageAlphaNone:kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big):NULL;
 if(!context){CGImageRelease(image);return dgsFail(r,"bitmap allocation failed");}
 if(r.gray){CGContextSetGrayFillColor(context,background,1);CGContextFillRect(context,CGRectMake(0,0,r.width,r.height));}
 CGContextTranslateCTM(context,0,r.height);CGContextScaleCTM(context,1,-1);CGContextDrawImage(context,CGRectMake(0,0,r.width,r.height),image);CGContextRelease(context);CGImageRelease(image);return r;
}
// dgsSRGB converts opaque 8-bit pixels described by icc (RGBA, or one gray
// byte per pixel) into sRGB RGBA through ColorSync. NULL icc returns the sRGB
// profile alone.
typedef struct {unsigned char *pixels,*icc; size_t iccLen; const char *error;} DGSConverted;
static DGSConverted dgsSRGB(const unsigned char *pixels,size_t width,size_t height,const void *icc,size_t iccLen,int gray){
 DGSConverted r={0};
 CGColorSpaceRef target=CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
 if(!target){r.error="sRGB colour space is unavailable";return r;}
 CFDataRef profile=CGColorSpaceCopyICCData(target);
 if(!profile){CGColorSpaceRelease(target);r.error="sRGB profile is unavailable";return r;}
 r.iccLen=CFDataGetLength(profile);r.icc=malloc(r.iccLen);
 if(!r.icc){CFRelease(profile);CGColorSpaceRelease(target);r.error="allocation failed";return r;}
 memcpy(r.icc,CFDataGetBytePtr(profile),r.iccLen);CFRelease(profile);
 if(!icc){CGColorSpaceRelease(target);return r;}
 CFDataRef data=CFDataCreate(NULL,icc,iccLen);CGColorSpaceRef source=CGColorSpaceCreateWithICCData(data);CFRelease(data);
 if(!source||CGColorSpaceGetModel(source)!=(gray?kCGColorSpaceModelMonochrome:kCGColorSpaceModelRGB)){
  if(source)CGColorSpaceRelease(source);CGColorSpaceRelease(target);free(r.icc);r.icc=NULL;r.error="ICC profile cannot describe these pixels";return r;}
 size_t channels=gray?1:4;
 CGDataProviderRef provider=CGDataProviderCreateWithData(NULL,pixels,width*height*channels,NULL);
 CGImageRef image=CGImageCreate(width,height,8,8*channels,width*channels,source,gray?kCGImageAlphaNone:(kCGImageAlphaNoneSkipLast|kCGBitmapByteOrder32Big),provider,NULL,false,kCGRenderingIntentDefault);
 CGDataProviderRelease(provider);CGColorSpaceRelease(source);
 r.pixels=image?calloc(width*height,4):NULL;
 CGContextRef context=r.pixels?CGBitmapContextCreate(r.pixels,width,height,8,width*4,target,kCGImageAlphaNoneSkipLast|kCGBitmapByteOrder32Big):NULL;
 CGColorSpaceRelease(target);
 if(!context){if(image)CGImageRelease(image);free(r.pixels);free(r.icc);r.pixels=NULL;r.icc=NULL;r.error="colour conversion failed";return r;}
 CGContextSetRenderingIntent(context,kCGRenderingIntentDefault);
 CGContextDrawImage(context,CGRectMake(0,0,width,height),image);CGContextRelease(context);CGImageRelease(image);
 for(size_t i=3;i<width*height*4;i+=4)r.pixels[i]=255;
 return r;
}
*/
import "C"
import (
	"errors"
	"image"
	"image/color"
	"unsafe"
)

// nativeProblem is empty: ImageIO decodes HEIC/HEIF and TIFF on this build.
func nativeProblem(kind string) string { return "" }

// decodeNative decodes HEIC/HEIF (8-bit only, refusing HDR) and TIFF (8 or
// 16-bit integer, quantised to 8-bit in its own colour space) through ImageIO.
func decodeNative(kind string, data []byte, o Options) (image.Image, metadata, error) {
	fail := func(m metadata, msg string) (image.Image, metadata, error) {
		return nil, m, errors.New(kind + ": " + msg)
	}
	if len(data) == 0 {
		return fail(metadata{}, "empty file")
	}
	background, _ := BackgroundColor(o.Background)
	level := float64(color.GrayModel.Convert(background).(color.Gray).Y) / 255
	r := C.dgsDecode(unsafe.Pointer(&data[0]), C.size_t(len(data)), C.size_t(o.MaxPixels), C.int(boolInt(o.PreserveGPS)), C.int(boolInt(kind == "TIFF")), C.double(level))
	if r.pixels != nil {
		defer C.free(unsafe.Pointer(r.pixels))
	}
	if r.icc != nil {
		defer C.free(unsafe.Pointer(r.icc))
	}
	if r.error != nil {
		return fail(metadata{}, C.GoString(r.error))
	}
	w, h, gray := int(r.width), int(r.height), r.gray != 0
	channels := 4
	if gray {
		channels = 1
	}
	pixels := C.GoBytes(unsafe.Pointer(r.pixels), C.int(w*h*channels))
	m := metadata{orientation: int(r.orientation), date: C.GoString(&r.date[0])}
	if r.icc != nil {
		m.icc = C.GoBytes(unsafe.Pointer(r.icc), C.int(r.iccLen))
	}
	if err := checkICC(m.icc, gray); err != nil {
		return fail(m, err.Error())
	}
	if m.orientation < 1 || m.orientation > 8 {
		return fail(m, "invalid orientation")
	}
	if gray {
		return &image.Gray{Pix: pixels, Stride: w, Rect: image.Rect(0, 0, w, h)}, m, nil
	}
	// CoreGraphics returns premultiplied RGBA in the decoded RGB colour space.
	for i := 0; i < len(pixels); i += 4 {
		a := int(pixels[i+3])
		if a > 0 && a < 255 {
			for j := 0; j < 3; j++ {
				pixels[i+j] = uint8(min(255, (int(pixels[i+j])*255+a/2)/a))
			}
		}
	}
	return &image.NRGBA{Pix: pixels, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}, m, nil
}

// toSRGB converts opaque pixels described by icc to sRGB and returns the sRGB
// profile to embed. Untagged pixels are taken to be sRGB already.
func toSRGB(img *image.NRGBA, icc []byte) (*image.NRGBA, []byte, error) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var r C.DGSConverted
	if len(icc) == 0 {
		r = C.dgsSRGB(nil, 0, 0, nil, 0, 0)
	} else {
		gray := string(icc[16:20]) == "GRAY"
		pixels := img.Pix
		if gray {
			pixels = make([]byte, w*h)
			for i := range pixels {
				pixels[i] = img.Pix[4*i]
			}
		}
		r = C.dgsSRGB((*C.uchar)(unsafe.Pointer(&pixels[0])), C.size_t(w), C.size_t(h), unsafe.Pointer(&icc[0]), C.size_t(len(icc)), C.int(boolInt(gray)))
	}
	if r.pixels != nil {
		defer C.free(unsafe.Pointer(r.pixels))
	}
	if r.icc != nil {
		defer C.free(unsafe.Pointer(r.icc))
	}
	if r.error != nil {
		return nil, nil, errors.New("sRGB conversion: " + C.GoString(r.error))
	}
	profile := C.GoBytes(unsafe.Pointer(r.icc), C.int(r.iccLen))
	if r.pixels == nil {
		return img, profile, nil
	}
	out := &image.NRGBA{Pix: C.GoBytes(unsafe.Pointer(r.pixels), C.int(w*h*4)), Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
	return out, profile, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
