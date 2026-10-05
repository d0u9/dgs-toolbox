//go:build darwin && cgo

// The macOS side of package pdfpage: a page drawn on white at the
// size asked, turned by its /Rotate, and written as a JPEG by ImageIO.
// PDFKit draws the page, not Core Graphics alone: CGContextDrawPDFPage draws
// the page's content and no annotations, so a filled form's typed values,
// which live in its fields' appearances, would be lost.
#import <PDFKit/PDFKit.h>
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
    CGImageRef image = NULL;
    @autoreleasepool {
        NSURL *url = [NSURL fileURLWithFileSystemRepresentation:path isDirectory:NO relativeToURL:nil];
        PDFDocument *document = [[[PDFDocument alloc] initWithURL:url] autorelease];
        if (document == nil) return 1;
        if (number < 1 || (NSUInteger)number > document.pageCount) return 2;
        PDFPage *page = [document pageAtIndex:(NSUInteger)(number - 1)];
        if (page == nil) return 2;
        NSRect box = [page boundsForBox:kPDFDisplayBoxCropBox];
        int rotate = ((page.rotation % 360) + 360) % 360;
        CGFloat w = box.size.width, h = box.size.height;
        if (rotate == 90 || rotate == 270) { CGFloat t = w; w = h; h = t; }
        CGFloat scale = (CGFloat)longSide / (w > h ? w : h);
        size_t width = (size_t)(w * scale + 0.5), height = (size_t)(h * scale + 0.5);
        if (width < 1) width = 1;
        if (height < 1) height = 1;
        CGColorSpaceRef space = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
        CGContextRef context = CGBitmapContextCreate(NULL, width, height, 8, 0, space, (CGBitmapInfo)kCGImageAlphaNoneSkipLast);
        CGColorSpaceRelease(space);
        if (context == NULL) return 3;
        CGContextSetRGBFillColor(context, 1, 1, 1, 1);
        CGContextFillRect(context, CGRectMake(0, 0, width, height));
        CGContextSetInterpolationQuality(context, kCGInterpolationHigh);
        CGContextScaleCTM(context, scale, scale);
        // drawWithBox turns the page by its rotation and puts the box's
        // corner at the origin, as CGPDFPageGetDrawingTransform did.
        [page drawWithBox:kPDFDisplayBoxCropBox toContext:context];
        image = CGBitmapContextCreateImage(context);
        CGContextRelease(context);
    }
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
