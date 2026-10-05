// Render the installer artwork without external image tools.
import AppKit

let size = NSSize(width: 640, height: 400)
let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: 640, pixelsHigh: 400,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
    isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
)!
bitmap.size = size
let context = NSGraphicsContext(bitmapImageRep: bitmap)!.cgContext
// Use Finder's top-left coordinate system so the arrow aligns with the icons.
context.translateBy(x: 0, y: size.height)
context.scaleBy(x: 1, y: -1)
context.setFillColor(CGColor(gray: 1, alpha: 1))
context.fill(CGRect(origin: .zero, size: size))

// Cropped, softly shaded cubes echo the app mark. Keep the installation
// area clear; the artwork belongs at the edges rather than behind the icons.
func drawCube(center: CGPoint, radius: CGFloat) {
    let halfWidth = radius * sqrt(3) / 2
    let top = CGPoint(x: center.x, y: center.y - radius)
    let upperRight = CGPoint(x: center.x + halfWidth, y: center.y - radius / 2)
    let lowerRight = CGPoint(x: center.x + halfWidth, y: center.y + radius / 2)
    let bottom = CGPoint(x: center.x, y: center.y + radius)
    let lowerLeft = CGPoint(x: center.x - halfWidth, y: center.y + radius / 2)
    let upperLeft = CGPoint(x: center.x - halfWidth, y: center.y - radius / 2)
    let faces: [([CGPoint], CGFloat)] = [
        ([top, upperRight, center, upperLeft], 0.985),
        ([upperLeft, center, bottom, lowerLeft], 0.965),
        ([center, upperRight, lowerRight, bottom], 0.945),
    ]
    context.setLineWidth(0.8)
    context.setLineJoin(.round)
    context.setStrokeColor(CGColor(gray: 0.90, alpha: 1))
    for (points, shade) in faces {
        context.setFillColor(CGColor(gray: shade, alpha: 1))
        context.addLines(between: points)
        context.closePath()
        context.drawPath(using: .fillStroke)
    }
}
drawCube(center: CGPoint(x: 615, y: 22), radius: 145)
drawCube(center: CGPoint(x: 12, y: 408), radius: 150)

context.setStrokeColor(CGColor(gray: 0.13, alpha: 1))
context.setLineWidth(2.4)
context.setLineCap(.round)
context.setLineJoin(.round)
context.move(to: CGPoint(x: 283, y: 205))
context.addLine(to: CGPoint(x: 357, y: 205))
context.move(to: CGPoint(x: 348, y: 196))
context.addLine(to: CGPoint(x: 357, y: 205))
context.addLine(to: CGPoint(x: 348, y: 214))
context.strokePath()

let data = bitmap.representation(using: .tiff, properties: [:])!
try data.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
