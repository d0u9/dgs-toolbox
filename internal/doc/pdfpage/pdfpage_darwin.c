//go:build darwin && cgo

// The Core Graphics side of package pdfpage: a page drawn on white at the
// size asked, turned by its /Rotate, and written as a JPEG by ImageIO.
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <ImageIO/ImageIO.h>
#include <stdlib.h>
#include <string.h>

static CGPDFDocumentRef openPDF(const char *path) {
    CFURLRef url = CFURLCreateFromFileSystemRepresentation(NULL, (const UInt8 *)path, (CFIndex)strlen(path), false);
    if (url == NULL) return NULL;
    CGPDFDocumentRef document = CGPDFDocumentCreateWithURL(url);
    CFRelease(url);
    return document;
}

int dgs_pdf_count(const char *path) {
    CGPDFDocumentRef document = openPDF(path);
    if (document == NULL) return -1;
    int n = (int)CGPDFDocumentGetNumberOfPages(document);
    CGPDFDocumentRelease(document);
    return n;
}

// dgs_pdf_render answers 0 with the JPEG in out, which the caller frees; 1
// when the file is not a PDF; 2 when it has no such page; 3 when drawing or
// writing failed.
int dgs_pdf_render(const char *path, int number, int longSide, double quality, unsigned char **out, long *length) {
    CGPDFDocumentRef document = openPDF(path);
    if (document == NULL) return 1;
    CGPDFPageRef page = CGPDFDocumentGetPage(document, (size_t)number);
    if (page == NULL) {
        CGPDFDocumentRelease(document);
        return 2;
    }
    CGRect box = CGPDFPageGetBoxRect(page, kCGPDFCropBox);
    int rotate = ((CGPDFPageGetRotationAngle(page) % 360) + 360) % 360;
    CGFloat w = box.size.width, h = box.size.height;
    if (rotate == 90 || rotate == 270) { CGFloat t = w; w = h; h = t; }
    CGFloat scale = (CGFloat)longSide / (w > h ? w : h);
    size_t width = (size_t)(w * scale + 0.5), height = (size_t)(h * scale + 0.5);
    if (width < 1) width = 1;
    if (height < 1) height = 1;
    CGColorSpaceRef space = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
    CGContextRef context = CGBitmapContextCreate(NULL, width, height, 8, 0, space, kCGImageAlphaNoneSkipLast);
    CGColorSpaceRelease(space);
    if (context == NULL) {
        CGPDFDocumentRelease(document);
        return 3;
    }
    CGContextSetRGBFillColor(context, 1, 1, 1, 1);
    CGContextFillRect(context, CGRectMake(0, 0, width, height));
    CGContextSetInterpolationQuality(context, kCGInterpolationHigh);
    CGContextScaleCTM(context, scale, scale);
    CGAffineTransform fit = CGPDFPageGetDrawingTransform(page, kCGPDFCropBox, CGRectMake(0, 0, w, h), 0, true);
    CGContextConcatCTM(context, fit);
    CGContextClipToRect(context, box);
    CGContextDrawPDFPage(context, page);
    CGImageRef image = CGBitmapContextCreateImage(context);
    CGContextRelease(context);
    CGPDFDocumentRelease(document);
    if (image == NULL) return 3;

    CFMutableDataRef data = CFDataCreateMutable(NULL, 0);
    CGImageDestinationRef dest = CGImageDestinationCreateWithData(data, CFSTR("public.jpeg"), 1, NULL);
    int status = 3;
    if (dest != NULL) {
        CFNumberRef q = CFNumberCreate(NULL, kCFNumberDoubleType, &quality);
        const void *keys[] = { kCGImageDestinationLossyCompressionQuality };
        const void *values[] = { q };
        CFDictionaryRef options = CFDictionaryCreate(NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        CGImageDestinationAddImage(dest, image, options);
        if (CGImageDestinationFinalize(dest)) {
            *length = (long)CFDataGetLength(data);
            *out = malloc((size_t)*length);
            memcpy(*out, CFDataGetBytePtr(data), (size_t)*length);
            status = 0;
        }
        CFRelease(options);
        CFRelease(q);
        CFRelease(dest);
    }
    CFRelease(data);
    CGImageRelease(image);
    return status;
}
