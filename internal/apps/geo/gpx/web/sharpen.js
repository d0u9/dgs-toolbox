// sharpen applies a mild unsharp mask to RGBA pixels in place: each colour
// moves away from its 3×3 Gaussian blur by amount. It restores the edge
// contrast that scaling a photo down smooths away; flat areas are unchanged
// and alpha is kept.
export const SHARPEN_AMOUNT = 0.4;

export function sharpen(pixels, width, height, amount = SHARPEN_AMOUNT) {
  const source = Uint8ClampedArray.from(pixels);
  const at = (x, y, c) => source[(Math.min(height - 1, Math.max(0, y)) * width + Math.min(width - 1, Math.max(0, x))) * 4 + c];
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      for (let c = 0; c < 3; c++) {
        const blur = (
          at(x - 1, y - 1, c) + 2 * at(x, y - 1, c) + at(x + 1, y - 1, c) +
          2 * at(x - 1, y, c) + 4 * at(x, y, c) + 2 * at(x + 1, y, c) +
          at(x - 1, y + 1, c) + 2 * at(x, y + 1, c) + at(x + 1, y + 1, c)
        ) / 16;
        const value = at(x, y, c);
        pixels[(y * width + x) * 4 + c] = value + amount * (value - blur);
      }
    }
  }
  return pixels;
}
