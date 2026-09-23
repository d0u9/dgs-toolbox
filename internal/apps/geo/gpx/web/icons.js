// Row icons drawn alike: 16-unit boxes, 1.4 strokes, the text's colour, so a
// row's marks line up and read as one set.

const svg = (body, fill = "none") =>
  `<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="${fill}" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`;

export const DIAMOND = svg(`<path d="M8 2.5 13.5 8 8 13.5 2.5 8z"/>`, "currentColor");
export const DIAMOND_OPEN = svg(`<path d="M8 2.5 13.5 8 8 13.5 2.5 8z"/>`);
export const FOLDER = svg(`<path d="M2 4.5v7.5h12V6H8L6.5 4.5z"/>`);
export const CHEVRON = svg(`<path d="M6 3.5 10.5 8 6 12.5"/>`);
export const UP = svg(`<path d="M4 10 8 6 12 10"/>`);
export const DOWN = svg(`<path d="M4 6 8 10 12 6"/>`);
