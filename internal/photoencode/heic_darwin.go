//go:build darwin && cgo

package photoencode

/*
#cgo LDFLAGS: -framework ImageIO -framework CoreGraphics -framework CoreFoundation
#include <ImageIO/ImageIO.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>
typedef struct {unsigned char *pixels,*icc; size_t width,height,iccLen; int orientation; char date[64];const char *error;} DGSImage;
static DGSImage dgsDecode(const void *bytes,size_t length,size_t maxPixels,int preserveGPS){
 DGSImage r={0};r.orientation=1;
 CFDataRef data=CFDataCreate(NULL,bytes,length);CGImageSourceRef source=CGImageSourceCreateWithData(data,NULL);CFRelease(data);
 if(!source){r.error="cannot open HEIC";return r;}
 if(CGImageSourceGetCount(source)!=1){r.error="multi-image HEIC is not supported";CFRelease(source);return r;}
 CFDictionaryRef props=CGImageSourceCopyPropertiesAtIndex(source,0,NULL);
 if(!props){r.error="missing HEIC properties";CFRelease(source);return r;}
 CFNumberRef width=CFDictionaryGetValue(props,kCGImagePropertyPixelWidth),height=CFDictionaryGetValue(props,kCGImagePropertyPixelHeight);
 int w=0,h=0;if(width)CFNumberGetValue(width,kCFNumberIntType,&w);if(height)CFNumberGetValue(height,kCFNumberIntType,&h);
 if(w<=0||h<=0||(size_t)w>maxPixels/(size_t)h){r.error="HEIC exceeds configured pixel limit";CFRelease(props);CFRelease(source);return r;}
 CFNumberRef depth=CFDictionaryGetValue(props,kCGImagePropertyDepth);int bits=0;if(depth)CFNumberGetValue(depth,kCFNumberIntType,&bits);
 if(bits>8){r.error="high-bit-depth/HDR HEIC is not supported without colour conversion";CFRelease(props);CFRelease(source);return r;}
 CFNumberRef orientation=CFDictionaryGetValue(props,kCGImagePropertyOrientation);if(orientation)CFNumberGetValue(orientation,kCFNumberIntType,&r.orientation);
 CFDictionaryRef gps=CFDictionaryGetValue(props,kCGImagePropertyGPSDictionary);
 if(preserveGPS&&gps){r.error="HEIC GPS preservation is not supported yet; disable Preserve GPS";CFRelease(props);CFRelease(source);return r;}
 CFDictionaryRef exif=CFDictionaryGetValue(props,kCGImagePropertyExifDictionary);if(exif){CFStringRef date=CFDictionaryGetValue(exif,kCGImagePropertyExifDateTimeOriginal);if(date)CFStringGetCString(date,r.date,sizeof(r.date),kCFStringEncodingUTF8);}
 CGImageRef image=CGImageSourceCreateImageAtIndex(source,0,NULL);CFRelease(props);CFRelease(source);
 if(!image){r.error="HEIC decode failed";return r;}
 CGColorSpaceRef space=CGImageGetColorSpace(image);
 if(!space||CGColorSpaceGetModel(space)!=kCGColorSpaceModelRGB||CGImageGetBitsPerComponent(image)>8){r.error="HEIC colour representation is unsupported";CGImageRelease(image);return r;}
 CFDataRef profile=CGColorSpaceCopyICCData(space);if(!profile){r.error="HEIC decoder did not supply an RGB ICC profile";CGImageRelease(image);return r;}
 r.iccLen=CFDataGetLength(profile);if(r.iccLen<128||r.iccLen>4*1024*1024){r.error="HEIC ICC profile size is invalid";CFRelease(profile);CGImageRelease(image);return r;}r.icc=malloc(r.iccLen);if(!r.icc){r.error="HEIC allocation failed";CFRelease(profile);CGImageRelease(image);return r;}memcpy(r.icc,CFDataGetBytePtr(profile),r.iccLen);CFRelease(profile);
 r.width=CGImageGetWidth(image);r.height=CGImageGetHeight(image);
 if(r.width>maxPixels/r.height){r.error="HEIC decoded dimensions exceed limit";free(r.icc);r.icc=NULL;CGImageRelease(image);return r;}
 r.pixels=calloc(r.width*r.height,4);CGContextRef context=r.pixels?CGBitmapContextCreate(r.pixels,r.width,r.height,8,r.width*4,space,kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big):NULL;
 if(!context){r.error="HEIC bitmap allocation failed";free(r.icc);free(r.pixels);r.icc=NULL;r.pixels=NULL;CGImageRelease(image);return r;}
 CGContextTranslateCTM(context,0,r.height);CGContextScaleCTM(context,1,-1);CGContextDrawImage(context,CGRectMake(0,0,r.width,r.height),image);CGContextRelease(context);CGImageRelease(image);return r;
}
*/
import "C"
import (
	"errors"
	"image"
	"unsafe"
)

func decodeHEIC(data []byte, o Options) (image.Image, metadata, error) {
	if len(data) == 0 {
		return nil, metadata{}, errors.New("empty HEIC")
	}
	r := C.dgsDecode(unsafe.Pointer(&data[0]), C.size_t(len(data)), C.size_t(o.MaxPixels), C.int(boolInt(o.PreserveGPS)))
	if r.pixels != nil {
		defer C.free(unsafe.Pointer(r.pixels))
	}
	if r.icc != nil {
		defer C.free(unsafe.Pointer(r.icc))
	}
	if r.error != nil {
		return nil, metadata{}, errors.New(C.GoString(r.error))
	}
	w, h := int(r.width), int(r.height)
	pixels := C.GoBytes(unsafe.Pointer(r.pixels), C.int(w*h*4))
	m := metadata{icc: C.GoBytes(unsafe.Pointer(r.icc), C.int(r.iccLen)), orientation: int(r.orientation), date: C.GoString(&r.date[0])}
	if err := checkICC(m.icc); err != nil {
		return nil, m, err
	}
	if m.orientation < 1 || m.orientation > 8 {
		return nil, m, errors.New("invalid HEIC orientation")
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
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
