//go:build darwin && cgo

// The PDFKit and Vision side of package ocr. The answer is one JSON object:
// {"pages": [{"text": ..., "source": "text"|"recognised"}]}, or
// {"error": "..."}.
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>
#import <PDFKit/PDFKit.h>
#import <Vision/Vision.h>
#include <stdlib.h>
#include <string.h>

// A page with fewer characters than this in its text layer is taken to be a
// scan, perhaps with a stray header, and is recognised instead.
static const NSUInteger textLayerMinimum = 16;

// The longer side of a rendered page, in pixels: about 250 dpi for A4, enough
// for Vision to read small print on a card.
static const CGFloat renderLongSide = 2900;

static char *answer(NSDictionary *object) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:object options:0 error:nil];
    if (data == nil) {
        data = [@"{\"error\":\"text recognition: could not encode the answer\"}" dataUsingEncoding:NSUTF8StringEncoding];
    }
    char *out = malloc(data.length + 1);
    memcpy(out, data.bytes, data.length);
    out[data.length] = 0;
    return out;
}

static CGImageRef render(PDFPage *page) {
    CGRect bounds = [page boundsForBox:kPDFDisplayBoxMediaBox];
    CGFloat longSide = MAX(bounds.size.width, bounds.size.height);
    if (longSide <= 0) {
        return NULL;
    }
    CGFloat scale = renderLongSide / longSide;
    size_t width = (size_t)ceil(bounds.size.width * scale);
    size_t height = (size_t)ceil(bounds.size.height * scale);
    CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
    CGContextRef context = CGBitmapContextCreate(NULL, width, height, 8, 0, space,
                                                 (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
    CGColorSpaceRelease(space);
    if (context == NULL) {
        return NULL;
    }
    CGContextSetRGBFillColor(context, 1, 1, 1, 1);
    CGContextFillRect(context, CGRectMake(0, 0, width, height));
    CGContextScaleCTM(context, scale, scale);
    CGContextTranslateCTM(context, -bounds.origin.x, -bounds.origin.y);
    [page drawWithBox:kPDFDisplayBoxMediaBox toContext:context];
    CGImageRef image = CGBitmapContextCreateImage(context);
    CGContextRelease(context);
    return image;
}

// recognise reads a rendered page's lines, top to bottom and then left to
// right. Vision's origin is the bottom left.
static NSString *recognise(CGImageRef image, NSError **error) {
    VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] init];
    request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
    request.usesLanguageCorrection = YES;
    request.recognitionLanguages = @[ @"zh-Hans", @"zh-Hant", @"en-US" ];
    VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCGImage:image options:@{}];
    if (![handler performRequests:@[ request ] error:error]) {
        return nil;
    }
    NSArray<VNRecognizedTextObservation *> *lines = [request.results
        sortedArrayUsingComparator:^NSComparisonResult(VNRecognizedTextObservation *a, VNRecognizedTextObservation *b) {
            CGFloat ay = CGRectGetMidY(a.boundingBox), by = CGRectGetMidY(b.boundingBox);
            // Lines whose middles are within half a line of each other are one row.
            CGFloat tolerance = MIN(a.boundingBox.size.height, b.boundingBox.size.height) / 2;
            if (fabs(ay - by) > tolerance) {
                return ay > by ? NSOrderedAscending : NSOrderedDescending;
            }
            CGFloat ax = a.boundingBox.origin.x, bx = b.boundingBox.origin.x;
            return ax < bx ? NSOrderedAscending : (ax > bx ? NSOrderedDescending : NSOrderedSame);
        }];
    NSMutableArray<NSString *> *text = [NSMutableArray array];
    for (VNRecognizedTextObservation *line in lines) {
        VNRecognizedText *best = [[line topCandidates:1] firstObject];
        if (best != nil) {
            [text addObject:best.string];
        }
    }
    return [text componentsJoinedByString:@"\n"];
}

char *dgs_ocr_pdf(const char *path, int maxPages) {
    @autoreleasepool {
        NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
        PDFDocument *document = [[PDFDocument alloc] initWithURL:url];
        if (document == nil) {
            return answer(@{@"error" : @"text recognition: the file is not a PDF PDFKit can open"});
        }
        if (document.isLocked) {
            return answer(@{@"error" : @"text recognition: the PDF is locked with a password"});
        }
        NSMutableArray *pages = [NSMutableArray array];
        NSInteger count = MIN((NSInteger)document.pageCount, (NSInteger)maxPages);
        for (NSInteger i = 0; i < count; i++) {
            @autoreleasepool {
                PDFPage *page = [document pageAtIndex:i];
                if (page == nil) {
                    continue;
                }
                NSString *layer = page.string;
                NSString *trimmed = [layer stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
                if (trimmed.length >= textLayerMinimum) {
                    [pages addObject:@{@"text" : layer, @"source" : @"text"}];
                    continue;
                }
                CGImageRef image = render(page);
                if (image == NULL) {
                    return answer(@{@"error" : [NSString stringWithFormat:@"text recognition: page %ld could not be drawn", (long)i + 1]});
                }
                NSError *error = nil;
                NSString *text = recognise(image, &error);
                CGImageRelease(image);
                if (text == nil) {
                    NSString *reason = error ? error.localizedDescription : @"unknown failure";
                    return answer(@{@"error" : [NSString stringWithFormat:@"text recognition: page %ld: %@", (long)i + 1, reason]});
                }
                [pages addObject:@{@"text" : text, @"source" : @"recognised"}];
            }
        }
        return answer(@{@"pages" : pages});
    }
}
