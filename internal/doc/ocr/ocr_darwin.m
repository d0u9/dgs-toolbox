//go:build darwin && cgo

// The PDFKit and Vision side of package ocr. One page is read per call, and
// the answer is one JSON object: {"count": pages, "page": {"source":
// "text"|"recognised", "lines": [...]}}, or {"error": "..."}. A line is its text and its box as fractions of the page,
// from the bottom left: of the page as shown for recognised lines
// ("space": "display"), of the unturned page for the text layer's ("space":
// "page", with the page's rotation). Go turns both into one frame.
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

// render draws the page as it is shown, turned by its /Rotate.
static CGImageRef render(PDFPage *page) {
    CGPDFPageRef source = page.pageRef;
    if (source == NULL) {
        return NULL;
    }
    CGRect media = CGPDFPageGetBoxRect(source, kCGPDFMediaBox);
    int rotate = ((CGPDFPageGetRotationAngle(source) % 360) + 360) % 360;
    CGFloat shownWidth = (rotate == 90 || rotate == 270) ? media.size.height : media.size.width;
    CGFloat shownHeight = (rotate == 90 || rotate == 270) ? media.size.width : media.size.height;
    CGFloat longSide = MAX(shownWidth, shownHeight);
    if (longSide <= 0) {
        return NULL;
    }
    CGFloat scale = renderLongSide / longSide;
    size_t width = (size_t)ceil(shownWidth * scale);
    size_t height = (size_t)ceil(shownHeight * scale);
    CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
    CGContextRef context = CGBitmapContextCreate(NULL, width, height, 8, 0, space,
                                                 (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
    CGColorSpaceRelease(space);
    if (context == NULL) {
        return NULL;
    }
    CGContextSetRGBFillColor(context, 1, 1, 1, 1);
    CGContextFillRect(context, CGRectMake(0, 0, width, height));
    // The drawing transform turns the page and fits it to a rectangle of its
    // own size in points; it never enlarges, so the scale is applied first.
    CGContextScaleCTM(context, scale, scale);
    CGAffineTransform fit = CGPDFPageGetDrawingTransform(source, kCGPDFMediaBox,
                                                         CGRectMake(0, 0, shownWidth, shownHeight), 0, true);
    CGContextConcatCTM(context, fit);
    CGContextClipToRect(context, media);
    CGContextDrawPDFPage(context, source);
    CGImageRef image = CGBitmapContextCreateImage(context);
    CGContextRelease(context);
    return image;
}

// recognise reads a rendered page's lines, top to bottom and then left to
// right. Vision's boxes are fractions of the image from the bottom left.
static NSArray *recognise(CGImageRef image, NSError **error) {
    VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] init];
    request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
    request.usesLanguageCorrection = YES;
    request.recognitionLanguages = @[ @"zh-Hans", @"zh-Hant", @"en-US" ];
    VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCGImage:image options:@{}];
    if (![handler performRequests:@[ request ] error:error]) {
        return nil;
    }
    NSArray<VNRecognizedTextObservation *> *found = [request.results
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
    NSMutableArray *lines = [NSMutableArray array];
    for (VNRecognizedTextObservation *line in found) {
        VNRecognizedText *best = [[line topCandidates:1] firstObject];
        if (best == nil) {
            continue;
        }
        CGRect box = line.boundingBox;
        [lines addObject:@{
            @"text" : best.string, @"space" : @"display",
            @"x" : @(box.origin.x), @"y" : @(box.origin.y), @"w" : @(box.size.width), @"h" : @(box.size.height),
        }];
    }
    return lines;
}

// layer reads the page's own text line by line, each with its box as a
// fraction of the unturned media box.
static NSArray *layer(PDFPage *page) {
    NSMutableArray *lines = [NSMutableArray array];
    if (page.numberOfCharacters == 0) {
        return lines;
    }
    CGRect media = [page boundsForBox:kPDFDisplayBoxMediaBox];
    if (media.size.width <= 0 || media.size.height <= 0) {
        return lines;
    }
    int rotate = (int)page.rotation;
    PDFSelection *all = [page selectionForRange:NSMakeRange(0, page.numberOfCharacters)];
    for (PDFSelection *line in all.selectionsByLine) {
        NSString *text = line.string;
        if (text.length == 0) {
            continue;
        }
        CGRect box = [line boundsForPage:page];
        [lines addObject:@{
            @"text" : text, @"space" : @"page", @"rotate" : @(rotate),
            @"x" : @((box.origin.x - media.origin.x) / media.size.width),
            @"y" : @((box.origin.y - media.origin.y) / media.size.height),
            @"w" : @(box.size.width / media.size.width),
            @"h" : @(box.size.height / media.size.height),
        }];
    }
    return lines;
}

char *dgs_ocr_page(const char *path, int index) {
    @autoreleasepool {
        NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
        PDFDocument *document = [[PDFDocument alloc] initWithURL:url];
        if (document == nil) {
            return answer(@{@"error" : @"text recognition: the file is not a PDF PDFKit can open"});
        }
        if (document.isLocked) {
            return answer(@{@"error" : @"text recognition: the PDF is locked with a password"});
        }
        NSInteger count = (NSInteger)document.pageCount;
        if (index < 0 || index >= count) {
            return answer(@{@"count" : @(count)});
        }
        PDFPage *page = [document pageAtIndex:index];
        if (page == nil) {
            return answer(@{@"count" : @(count)});
        }
        NSString *own = page.string;
        NSString *trimmed = [own stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
        if (trimmed.length >= textLayerMinimum) {
            return answer(@{@"count" : @(count), @"page" : @{@"lines" : layer(page), @"source" : @"text"}});
        }
        CGImageRef image = render(page);
        if (image == NULL) {
            return answer(@{@"error" : [NSString stringWithFormat:@"text recognition: page %d could not be drawn", index + 1]});
        }
        NSError *error = nil;
        NSArray *lines = recognise(image, &error);
        CGImageRelease(image);
        if (lines == nil) {
            NSString *reason = error ? error.localizedDescription : @"unknown failure";
            return answer(@{@"error" : [NSString stringWithFormat:@"text recognition: page %d: %@", index + 1, reason]});
        }
        return answer(@{@"count" : @(count), @"page" : @{@"lines" : lines, @"source" : @"recognised"}});
    }
}
