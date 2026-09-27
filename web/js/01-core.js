// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot
// Sakura Print web app, part 01: Icons, illustrations, app state, talking to the server, theme, printer options, routing.
'use strict';

const $ = (s, r = document) => r.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

// ---------- icons (simple line drawings) ----------
// icons: soft-filled bodies with a clear outline, so every shape has an inside
const I = {
  "doc": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M6.5 2.5h8L19 7v13a1.5 1.5 0 01-1.5 1.5h-11A1.5 1.5 0 015 20V4a1.5 1.5 0 011.5-1.5z\"/><path d=\"M6.5 2.5h8L19 7v13a1.5 1.5 0 01-1.5 1.5h-11A1.5 1.5 0 015 20V4a1.5 1.5 0 011.5-1.5z\"/><path d=\"M14.5 2.5V7H19\"/><path d=\"M8.5 12h7M8.5 15.5h7M8.5 19h4\"/>",
  "photo": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"3\" y=\"4.5\" width=\"18\" height=\"15\" rx=\"2.5\"/><rect x=\"3\" y=\"4.5\" width=\"18\" height=\"15\" rx=\"2.5\"/><circle fill=\"currentColor\" cx=\"8.5\" cy=\"9.5\" r=\"1.8\"/><path fill=\"currentColor\" d=\"M3.8 18.2l5-5.2 3.7 3.8 2.6-2.7 5.1 5.1v.3H3.8z\" fill-opacity=\".85\"/>",
  "scan": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"3\" y=\"12.5\" width=\"18\" height=\"8\" rx=\"2\"/><rect x=\"3\" y=\"12.5\" width=\"18\" height=\"8\" rx=\"2\"/><path fill=\"currentColor\" fill-opacity=\".55\" d=\"M3.6 11.2L18.6 4.3a1 1 0 011.3.5l.4.9a1 1 0 01-.5 1.3L5.8 13.2z\"/><path d=\"M3.6 11.2L18.6 4.3a1 1 0 011.3.5l.4.9a1 1 0 01-.5 1.3L5.8 13.2\"/><path stroke-width=\"2.4\" d=\"M7 16.5h10\"/>",
  "copy": "<rect x=\"3.5\" y=\"3\" width=\"11\" height=\"14\" rx=\"2\"/><rect fill=\"currentColor\" x=\"9.5\" y=\"7\" width=\"11\" height=\"14\" rx=\"2\" fill-opacity=\".35\"/><rect x=\"9.5\" y=\"7\" width=\"11\" height=\"14\" rx=\"2\"/><path d=\"M12.5 12h5M12.5 15.5h5\"/>",
  "printer": "<path d=\"M7 8V3.5h10V8\"/><rect fill=\"currentColor\" fill-opacity=\".22\" x=\"2.5\" y=\"8\" width=\"19\" height=\"9\" rx=\"2.5\"/><rect x=\"2.5\" y=\"8\" width=\"19\" height=\"9\" rx=\"2.5\"/><rect fill=\"currentColor\" x=\"7\" y=\"13.5\" width=\"10\" height=\"7.5\" rx=\"1\" fill-opacity=\".5\"/><rect x=\"7\" y=\"13.5\" width=\"10\" height=\"7.5\" rx=\"1\"/><circle fill=\"currentColor\" cx=\"17.5\" cy=\"11\" r=\"1.1\"/>",
  "gear": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M12.00 4.60 L13.87 2.58 L15.67 3.13 L16.11 5.85 L17.23 6.77 L19.98 6.67 L20.87 8.33 L19.26 10.56 L19.40 12.00 L21.42 13.87 L20.87 15.67 L18.15 16.11 L17.23 17.23 L17.33 19.98 L15.67 20.87 L13.44 19.26 L12.00 19.40 L10.13 21.42 L8.33 20.87 L7.89 18.15 L6.77 17.23 L4.02 17.33 L3.13 15.67 L4.74 13.44 L4.60 12.00 L2.58 10.13 L3.13 8.33 L5.85 7.89 L6.77 6.77 L6.67 4.02 L8.33 3.13 L10.56 4.74Z\"/><path stroke-linejoin=\"round\" d=\"M12.00 4.60 L13.87 2.58 L15.67 3.13 L16.11 5.85 L17.23 6.77 L19.98 6.67 L20.87 8.33 L19.26 10.56 L19.40 12.00 L21.42 13.87 L20.87 15.67 L18.15 16.11 L17.23 17.23 L17.33 19.98 L15.67 20.87 L13.44 19.26 L12.00 19.40 L10.13 21.42 L8.33 20.87 L7.89 18.15 L6.77 17.23 L4.02 17.33 L3.13 15.67 L4.74 13.44 L4.60 12.00 L2.58 10.13 L3.13 8.33 L5.85 7.89 L6.77 6.77 L6.67 4.02 L8.33 3.13 L10.56 4.74Z\"/><circle cx=\"12\" cy=\"12\" r=\"3\"/>",
  "upload": "<path d=\"M12 15V4.5\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M12.00 3.50L13.60 6.27L10.40 6.27Z\"/><path fill=\"currentColor\" fill-opacity=\".22\" d=\"M4 14v4.5A2.5 2.5 0 006.5 21h11a2.5 2.5 0 002.5-2.5V14\"/><path d=\"M4 14v4.5A2.5 2.5 0 006.5 21h11a2.5 2.5 0 002.5-2.5V14\"/>",
  "folder": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M3 7a2 2 0 012-2h4.2l2 2.2H19a2 2 0 012 2V18a2 2 0 01-2 2H5a2 2 0 01-2-2z\"/><path d=\"M3 7a2 2 0 012-2h4.2l2 2.2H19a2 2 0 012 2V18a2 2 0 01-2 2H5a2 2 0 01-2-2z\"/><path d=\"M3 10h18\"/>",
  "camera": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M3 8.5a2 2 0 012-2h2.5L9 4h6l1.5 2.5H19a2 2 0 012 2V18a2 2 0 01-2 2H5a2 2 0 01-2-2z\"/><path d=\"M3 8.5a2 2 0 012-2h2.5L9 4h6l1.5 2.5H19a2 2 0 012 2V18a2 2 0 01-2 2H5a2 2 0 01-2-2z\"/><circle cx=\"12\" cy=\"13\" r=\"3.8\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"13\" r=\"1.6\"/>",
  "edit": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M4 20l1-4.5L15.5 5a2.1 2.1 0 013 3L8 18.5z\"/><path d=\"M4 20l1-4.5L15.5 5a2.1 2.1 0 013 3L8 18.5z\"/><path d=\"M13.5 7l3 3\"/><path d=\"M4 20h16\"/>",
  "x": "<path stroke-width=\"2.4\" d=\"M6.5 6.5l11 11M17.5 6.5l-11 11\"/>",
  "info": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"12\" cy=\"12\" r=\"9\"/><circle cx=\"12\" cy=\"12\" r=\"9\"/><path stroke-width=\"2.4\" d=\"M12 11v6\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"7.6\" r=\"1.2\"/>",
  "plus": "<path stroke-width=\"2.4\" d=\"M12 5v14M5 12h14\"/>",
  "up": "<path stroke-width=\"2.4\" d=\"M6 14.5l6-6 6 6\"/>",
  "down": "<path stroke-width=\"2.4\" d=\"M6 9.5l6 6 6-6\"/>",
  "turn": "<path d=\"M19.5 12a7.5 7.5 0 11-2.6-5.7\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M19.60 6.80L18.08 10.06L16.01 7.11Z\"/>",
  "turnL": "<path d=\"M4.5 12a7.5 7.5 0 102.6-5.7\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M4.40 6.80L7.99 7.11L5.92 10.06Z\"/>",
  "undo": "<path d=\"M5 9h9.5a5.5 5.5 0 010 11H10\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M3.60 9.00L6.89 7.10L6.89 10.90Z\"/>",
  "check": "<path stroke-width=\"2.6\" d=\"M5 12.5l4.5 4.5L19 7\"/>",
  "crop": "<path stroke-width=\"2.3\" d=\"M6.5 2.5V15a2 2 0 002 2h13\"/><path stroke-width=\"2.3\" d=\"M17.5 21.5V9a2 2 0 00-2-2h-13\"/>",
  "level": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"5\" y=\"5\" width=\"14\" height=\"14\" rx=\"1.5\" transform=\"rotate(-12 12 12)\"/><rect x=\"5\" y=\"5\" width=\"14\" height=\"14\" rx=\"1.5\" transform=\"rotate(-12 12 12)\"/><path stroke-dasharray=\"2 2.5\" d=\"M2 20.5h20\"/>",
  "pen": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M4 20l1.2-4.7L16 4.5l3.5 3.5L8.7 18.8z\"/><path d=\"M4 20l1.2-4.7L16 4.5l3.5 3.5L8.7 18.8z\"/><path d=\"M14 6.5l3.5 3.5\"/>",
  "marker": "<path fill=\"currentColor\" d=\"M15 3.5l5.5 5.5-8 8H7v-5.5z\" fill-opacity=\".45\"/><path d=\"M15 3.5l5.5 5.5-8 8H7v-5.5z\"/><path d=\"M4 21h9\"/><path d=\"M7 17l-2 2\"/>",
  "text": "<path stroke-width=\"2.4\" d=\"M5 7V4.5h14V7M12 4.5v15M9 19.5h6\"/>",
  "phone": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"6\" y=\"2\" width=\"12\" height=\"20\" rx=\"2.8\"/><rect x=\"6\" y=\"2\" width=\"12\" height=\"20\" rx=\"2.8\"/><path stroke-width=\"2.4\" d=\"M10.5 18.5h3\"/>",
  "download": "<path d=\"M12 3.5V14\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M12.00 15.20L10.40 12.43L13.60 12.43Z\"/><path d=\"M4 20.5h16\"/>",
  "clip": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M6 5h12a1 1 0 011 1v14a1.5 1.5 0 01-1.5 1.5h-11A1.5 1.5 0 015 20V6a1 1 0 011-1z\"/><path d=\"M8 5H6a1 1 0 00-1 1v14a1.5 1.5 0 001.5 1.5h11A1.5 1.5 0 0019 20V6a1 1 0 00-1-1h-2\"/><rect fill=\"currentColor\" x=\"8\" y=\"2.5\" width=\"8\" height=\"4.5\" rx=\"1.2\" fill-opacity=\".9\"/><path d=\"M8.5 12h7M8.5 15.5h5\"/>",
  "book": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M12 6.5C10 5 7 4.5 3 5v14c4-.5 7 0 9 1.5 2-1.5 5-2 9-1.5V5c-4-.5-7 0-9 1.5z\"/><path d=\"M12 6.5C10 5 7 4.5 3 5v14c4-.5 7 0 9 1.5 2-1.5 5-2 9-1.5V5c-4-.5-7 0-9 1.5z\"/><path d=\"M12 6.5v14\"/>",
  "move": "<path d=\"M12 5v14M5 12h14\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M12.00 3.00L13.70 5.94L10.30 5.94Z\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M12.00 21.00L10.30 18.06L13.70 18.06Z\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M3.00 12.00L5.94 10.30L5.94 13.70Z\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M21.00 12.00L18.06 13.70L18.06 10.30Z\"/>",
  "fill": "<rect fill=\"currentColor\" x=\"4\" y=\"4\" width=\"16\" height=\"16\" rx=\"2.5\"/>",
  "whole": "<rect x=\"4\" y=\"4\" width=\"16\" height=\"16\" rx=\"2.5\"/><rect fill=\"currentColor\" x=\"7.5\" y=\"8.5\" width=\"9\" height=\"7\" rx=\"1.2\" fill-opacity=\".6\"/>",
  "expand": "<path stroke-width=\"2.3\" d=\"M4 9.5V4h5.5M20 9.5V4h-5.5M4 14.5V20h5.5M20 14.5V20h-5.5\"/>",
  "help": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"12\" cy=\"12\" r=\"9.5\"/><circle cx=\"12\" cy=\"12\" r=\"9.5\"/><path stroke-width=\"2.3\" d=\"M9.4 9.4a2.7 2.7 0 115.2 1c-.6 1.2-2.6 1.6-2.6 3.2\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"17.2\" r=\"1.3\"/>",
  "lock": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"4.5\" y=\"10.5\" width=\"15\" height=\"11\" rx=\"2.5\"/><rect x=\"4.5\" y=\"10.5\" width=\"15\" height=\"11\" rx=\"2.5\"/><path d=\"M8 10.5V7.5a4 4 0 018 0v3\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"16\" r=\"1.6\"/>",
  "wrench": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M14.7 5.3a4.5 4.5 0 00-5.8 5.8L3.5 16.5a2.1 2.1 0 003 3l5.4-5.4a4.5 4.5 0 005.8-5.8l-2.9 2.9-2.5-.5-.5-2.5z\"/><path d=\"M14.7 5.3a4.5 4.5 0 00-5.8 5.8L3.5 16.5a2.1 2.1 0 003 3l5.4-5.4a4.5 4.5 0 005.8-5.8l-2.9 2.9-2.5-.5-.5-2.5z\"/>",
  "home": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M3.5 11L12 4l8.5 7v9a1.5 1.5 0 01-1.5 1.5h-4.5v-6h-5v6H5A1.5 1.5 0 013.5 20z\"/><path d=\"M3.5 11L12 4l8.5 7v9a1.5 1.5 0 01-1.5 1.5h-4.5v-6h-5v6H5A1.5 1.5 0 013.5 20z\"/>",
  "text2": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M4 4.5h16v15H4z\"/><path d=\"M4 4.5h16v15H4z\"/><path d=\"M7.5 9h9M7.5 12.5h9M7.5 16h5\"/>",
  "eye": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M2 12s3.8-7 10-7 10 7 10 7-3.8 7-10 7S2 12 2 12z\"/><path d=\"M2 12s3.8-7 10-7 10 7 10 7-3.8 7-10 7S2 12 2 12z\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"12\" r=\"3.2\"/>",
  "trash": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M6 7h12l-1 13a1.5 1.5 0 01-1.5 1.4h-7A1.5 1.5 0 017 20z\"/><path d=\"M6 7h12l-1 13a1.5 1.5 0 01-1.5 1.4h-7A1.5 1.5 0 017 20z\"/><path d=\"M4 7h16M9.5 7V4.5h5V7M10 11v6.5M14 11v6.5\"/>",
  "redo": "<path d=\"M19 9H9.5a5.5 5.5 0 000 11H14\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M20.40 9.00L17.11 10.90L17.11 7.10Z\"/>",
  "pointer": "<path fill=\"currentColor\" fill-opacity=\".22\" d=\"M5 3.5l13 7.2-5.6 1.6 3.4 6.4-2.6 1.4-3.4-6.4L5.5 17.5z\"/><path stroke-linejoin=\"round\" d=\"M5 3.5l13 7.2-5.6 1.6 3.4 6.4-2.6 1.4-3.4-6.4L5.5 17.5z\"/>",
  "shapes": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"3\" y=\"11\" width=\"10\" height=\"10\" rx=\"1.5\"/><rect x=\"3\" y=\"11\" width=\"10\" height=\"10\" rx=\"1.5\"/><circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"16\" cy=\"8\" r=\"5\"/><circle cx=\"16\" cy=\"8\" r=\"5\"/>",
  "arrow": "<path stroke-width=\"2.4\" d=\"M5 19L18 6\"/><path fill=\"currentColor\" stroke-linejoin=\"round\" d=\"M19.5 4.5L18.3 11.2L12.8 5.7Z\"/>",
  "line": "<path stroke-width=\"2.4\" d=\"M5 19L19 5\"/>",
  "rect": "<rect fill=\"currentColor\" fill-opacity=\".22\" x=\"4\" y=\"6\" width=\"16\" height=\"12\" rx=\"1.5\"/><rect x=\"4\" y=\"6\" width=\"16\" height=\"12\" rx=\"1.5\"/>",
  "circle": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"12\" cy=\"12\" r=\"8\"/><circle cx=\"12\" cy=\"12\" r=\"8\"/>",
  "dial": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"12\" cy=\"12\" r=\"9\"/><circle cx=\"12\" cy=\"12\" r=\"9\"/><path stroke-width=\"2.3\" d=\"M12 12l4.5-4.5\"/><circle fill=\"currentColor\" cx=\"12\" cy=\"12\" r=\"1.8\"/><path d=\"M12 3v2.2M21 12h-2.2M3 12h2.2M5.6 5.6l1.6 1.6\"/>",
  "filters": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"9\" cy=\"9\" r=\"5.5\"/><circle cx=\"9\" cy=\"9\" r=\"5.5\"/><circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"15\" cy=\"9\" r=\"5.5\"/><circle cx=\"15\" cy=\"9\" r=\"5.5\"/><circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"12\" cy=\"15\" r=\"5.5\"/><circle cx=\"12\" cy=\"15\" r=\"5.5\"/>",
  "flip": "<path stroke-dasharray=\"2 2.2\" d=\"M12 3v18\"/><path fill=\"currentColor\" fill-opacity=\".22\" stroke-linejoin=\"round\" d=\"M9.5 6v12L3 18z\"/><path stroke-linejoin=\"round\" d=\"M9.5 6v12L3 18z\"/><path stroke-linejoin=\"round\" d=\"M14.5 6v12L21 18z\"/>",
  "eraser": "<path fill=\"currentColor\" fill-opacity=\".22\" stroke-linejoin=\"round\" d=\"M14.2 4.3l5.5 5.5L10 19.5H6.5l-3-3z\"/><path stroke-linejoin=\"round\" d=\"M14.2 4.3l5.5 5.5L10 19.5H6.5l-3-3z\"/><path d=\"M8.5 10l5.5 5.5M10 19.5h10\"/>",
  "sign": "<path stroke-width=\"2\" d=\"M3 16c2-6 4-9 5.5-9 2.3 0-1.5 10 1 10 1.6 0 2.3-4 3.8-4s.9 3 2.4 3c1 0 1.8-.8 2.3-1.8\"/><path d=\"M3 20.5h18\"/>",
  "mag": "<circle fill=\"currentColor\" fill-opacity=\".22\" cx=\"10.5\" cy=\"10.5\" r=\"6.5\"/><circle cx=\"10.5\" cy=\"10.5\" r=\"6.5\"/><path stroke-width=\"2.6\" d=\"M15.5 15.5L21 21\"/><path d=\"M8 10.5h5M10.5 8v5\"/>",
  "bubble": "<path fill=\"currentColor\" fill-opacity=\".22\" stroke-linejoin=\"round\" d=\"M5 4h14a2 2 0 012 2v9a2 2 0 01-2 2h-8l-5 4v-4H5a2 2 0 01-2-2V6a2 2 0 012-2z\"/><path stroke-linejoin=\"round\" d=\"M5 4h14a2 2 0 012 2v9a2 2 0 01-2 2h-8l-5 4v-4H5a2 2 0 01-2-2V6a2 2 0 012-2z\"/><path d=\"M7.5 9h9M7.5 12.5h6\"/>",
  "star": "<path fill=\"currentColor\" fill-opacity=\".22\" stroke-linejoin=\"round\" d=\"M12 3l2.7 5.8 6.3.7-4.7 4.3 1.3 6.2L12 16.9 6.4 20l1.3-6.2L3 9.5l6.3-.7z\"/><path stroke-linejoin=\"round\" d=\"M12 3l2.7 5.8 6.3.7-4.7 4.3 1.3 6.2L12 16.9 6.4 20l1.3-6.2L3 9.5l6.3-.7z\"/>",
  "drop": "<path fill=\"currentColor\" fill-opacity=\".22\" stroke-linejoin=\"round\" d=\"M14.5 5.5l4 4-9 9-4.5.5.5-4.5z\"/><path stroke-linejoin=\"round\" d=\"M14.5 5.5l4 4-9 9-4.5.5.5-4.5z\"/><path stroke-width=\"2.4\" d=\"M13 4l7 7M16 3.5l4.5 4.5\"/>",
  "go": "<path d=\"M7 8V3.5h10V8\"/><rect fill=\"currentColor\" fill-opacity=\".22\" x=\"2.5\" y=\"8\" width=\"19\" height=\"9\" rx=\"2.5\"/><rect x=\"2.5\" y=\"8\" width=\"19\" height=\"9\" rx=\"2.5\"/><rect x=\"7\" y=\"13.5\" width=\"10\" height=\"7.5\" rx=\"1\"/>",
};
const icon = (n) => `<svg viewBox="0 0 24 24">${I[n]}</svg>`;
// ic(): an inline icon that sits in a line of text or on a button
const ic = n => `<svg class="ic" viewBox="0 0 24 24" aria-hidden="true">${I[n]}</svg>`;
// the drawn flower that marks the top of a page (matches the illustrations and the printed test pages)
const FLOWER = `<svg class="ic fl-ic" viewBox="0 0 24 24" aria-label="flower">${[0, 72, 144, 216, 288].map(a =>
  `<ellipse cx="12" cy="6.6" rx="3.4" ry="4.8" transform="rotate(${a} 12 12)"/>`).join('')}<circle cx="12" cy="12" r="2.4" class="fl-mid"/></svg>`;
// a big round icon at the top of popups
const bigIcon = n => `<div class="bigicon">${icon(n)}</div>`;

// every feature has its own colour, used for its tile and its page header
const F = {
  docs:     { ic: 'doc',     t: 'Documents',    s: 'PDFs, one or both sides',   g: ['#f59ab8', '#e0648f'] },
  photos:   { ic: 'photo',   t: 'Photos',       s: 'Pictures from your phone',  g: ['#8dbcf3', '#5a8fe0'] },
  text:     { ic: 'text2',    t: 'Text',         s: 'Paste or type anything',    g: ['#bda5f2', '#8f73dc'] },
  copy:     { ic: 'copy',    t: 'Copy',         s: 'Scan and print at once',    g: ['#f8b98a', '#ea8a4c'] },
  scan:     { ic: 'scan',    t: 'Scan',         s: 'Paper to PDF or pictures',  g: ['#86d0a9', '#43a878'] },
  printer:  { ic: 'printer', t: 'Printer care', s: 'Status, ink, cleaning',     g: ['#cfb094', '#a57c57'] },
  settings: { ic: 'gear',    t: 'Settings',     s: 'Colours, phone access',     g: ['#b3b3c4', '#7d7d93'] },
  again:    { ic: 'redo',    t: 'Print again',  s: 'Your recent prints',        g: ['#f0a6c8', '#c4679a'] },
  addprinter: { ic: 'printer', t: 'Printers & drivers', s: 'Set up a printer',     g: ['#9fc7c9', '#5f9ea3'] },
  advanced: { ic: 'gear',    t: 'Advanced',     s: 'Under the hood',            g: ['#a9b3c2', '#6f7c91'] },
  setup:    { ic: 'printer', t: 'Set up',       s: 'Getting ready',             g: ['#f59ab8', '#e0648f'] },
};
const grad = k => `--g1:${F[k].g[0]};--g2:${F[k].g[1]}`;
const pageHead = (k, sub, title) => `<div class="hero" style="${grad(k)}"><div class="hic">${icon(F[k].ic)}</div>
  <div><h1>${title || F[k].t}</h1><p>${sub}</p></div></div>`;

// ---------- illustrations: pictures do the explaining, text lives outside the drawing ----------
// badge(n): a big numbered circle; arrow(): a thick arrow with its own head (no shared SVG ids)
const badge = (x, y, n) => `<g><circle cx="${x}" cy="${y}" r="15" fill="var(--deep)" stroke="#fff" stroke-width="3"/>
  <text x="${x}" y="${y + 6}" text-anchor="middle" font-family="system-ui,sans-serif" font-size="17" font-weight="800" fill="#fff">${n}</text></g>`;
function arrow(x1, y1, x2, y2, bend = 0) {
  const mx = (x1 + x2) / 2 + bend, my = (y1 + y2) / 2 - Math.abs(bend) * 0.3;
  const a = Math.atan2(y2 - my, x2 - mx), L = 13, W = 9;
  const bx = x2 - Math.cos(a) * L, by = y2 - Math.sin(a) * L;
  const px = -Math.sin(a) * W, py = Math.cos(a) * W;
  return `<path d="M${x1} ${y1} Q${mx} ${my} ${bx} ${by}" fill="none" stroke="var(--acc2)" stroke-width="6" stroke-linecap="round"/>
    <polygon points="${x2},${y2} ${bx + px},${by + py} ${bx - px},${by - py}" fill="var(--acc2)"/>`;
}
const flower = (cx, cy, r) => `<g transform="translate(${cx} ${cy})">${[0, 72, 144, 216, 288].map(a =>
  `<ellipse cy="${-r * 0.55}" rx="${r * 0.42}" ry="${r * 0.6}" fill="var(--acc2)" transform="rotate(${a})"/>`).join('')}<circle r="${r * 0.3}" fill="#ffd166"/></g>`;
const sheet = (x, y, w, h, n, tilt = 0, blank = false) => `<g transform="rotate(${tilt} ${x + w / 2} ${y + h / 2})">
  <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="4" fill="${blank ? '#f3eff1' : '#fff'}" stroke="var(--deep)" stroke-width="2.5"/>
  ${n && h > 50 ? flower(x + w / 2, y + Math.min(16, h * 0.14), Math.min(10, w * 0.09)) : ''}
  ${n ? `<text x="${x + w / 2}" y="${y + h / 2 + (h > 50 ? 16 : 10)}" text-anchor="middle" font-family="system-ui,sans-serif" font-size="${Math.min(w, h) * 0.45}" font-weight="800" fill="var(--deep)">${n}</text>`
    : blank ? '' : `<path d="M${x + 10} ${y + 14}h${w - 20}M${x + 10} ${y + 24}h${w - 28}M${x + 10} ${y + 34}h${w - 24}" stroke="var(--acc)" stroke-width="4" stroke-linecap="round"/>`}</g>`;
const stack = (x, y, w, h, n) => `<rect x="${x + 8}" y="${y + 8}" width="${w}" height="${h}" rx="4" fill="#fff" stroke="var(--deep)" stroke-width="2" opacity=".45"/>
  <rect x="${x + 4}" y="${y + 4}" width="${w}" height="${h}" rx="4" fill="#fff" stroke="var(--deep)" stroke-width="2" opacity=".7"/>${sheet(x, y, w, h, n)}`;

// the two kinds of printer, seen from the front-left
function printerTray(extra = '') {
  return `<ellipse cx="160" cy="206" rx="128" ry="10" fill="rgba(0,0,0,.08)"/>
    <rect x="40" y="52" width="240" height="140" rx="20" fill="var(--deep)"/>
    <rect x="40" y="52" width="240" height="24" rx="12" fill="rgba(255,255,255,.18)"/>
    <circle cx="252" cy="96" r="7" fill="#7ee0a3"/>
    <rect x="78" y="104" width="164" height="10" rx="5" fill="rgba(0,0,0,.35)"/>
    <rect x="58" y="150" width="204" height="40" rx="8" fill="#fff" stroke="var(--deep)" stroke-width="3"/>
    <path d="M72 162h176M72 170h176M72 178h176" stroke="var(--acc)" stroke-width="3.5"/>
    <rect x="138" y="182" width="44" height="7" rx="3.5" fill="var(--deep)"/>${extra}`;
}
function printerRear(extra = '') {
  return `<ellipse cx="160" cy="206" rx="126" ry="10" fill="rgba(0,0,0,.08)"/>
    <g transform="rotate(-9 160 70)"><rect x="104" y="6" width="112" height="84" rx="4" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/>
      <rect x="98" y="14" width="112" height="84" rx="4" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/>
      <path d="M112 34h84M112 46h72M112 58h78" stroke="var(--acc)" stroke-width="4.5" stroke-linecap="round"/></g>
    <rect x="48" y="92" width="224" height="98" rx="20" fill="var(--deep)"/>
    <rect x="48" y="92" width="224" height="22" rx="11" fill="rgba(255,255,255,.18)"/>
    <circle cx="246" cy="130" r="7" fill="#7ee0a3"/>
    <rect x="88" y="164" width="144" height="10" rx="5" fill="rgba(0,0,0,.35)"/>${extra}`;
}

const ART = {
  // choice pictures, with numbered spots explained underneath
  tray: () => `<svg viewBox="0 0 320 220" class="art">${printerTray(`
      ${sheet(96, 90, 128, 36, '', 0)}
      ${badge(262, 170, 1)}${badge(242, 118, 2)}`)}</svg>`,
  rear: () => `<svg viewBox="0 0 320 220" class="art">${printerRear(`
      <path d="M98 170h124v26a5 5 0 0 1-5 5H103a5 5 0 0 1-5-5z" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/>
      ${badge(232, 30, 1)}${badge(238, 190, 2)}`)}</svg>`,
  faceUp: () => `<svg viewBox="0 0 320 200" class="art"><rect x="40" y="130" width="240" height="16" rx="8" fill="var(--deep)" opacity=".9"/>
      <g transform="translate(160 86) scale(1 .6) rotate(-5)">${sheet(-75, -95, 150, 190, '')}</g>
      <circle cx="262" cy="46" r="26" fill="var(--soft)" stroke="var(--deep)" stroke-width="3"/><path d="M246 46q16-14 32 0q-16 14-32 0z" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/><circle cx="262" cy="46" r="5" fill="var(--deep)"/></svg>`,
  faceDown: () => `<svg viewBox="0 0 320 200" class="art"><rect x="40" y="130" width="240" height="16" rx="8" fill="var(--deep)" opacity=".9"/>
      <g transform="translate(160 86) scale(1 .6) rotate(-5)">${sheet(-75, -95, 150, 190, '', 0, true)}</g>
      <circle cx="262" cy="46" r="26" fill="var(--soft)" stroke="var(--deep)" stroke-width="3"/><path d="M246 46q16-14 32 0q-16 14-32 0z" fill="#fff" stroke="var(--deep)" stroke-width="2.5"/><circle cx="262" cy="46" r="5" fill="var(--deep)"/>
      <path d="M244 64l36-36" stroke="var(--deep)" stroke-width="4" stroke-linecap="round"/></svg>`,
};

// the "is the back the right way up?" answer, as pictures instead of words
const backPic = upside => `<svg viewBox="0 0 120 150" class="art"><g transform="rotate(${upside ? 180 : 0} 60 75)">${sheet(20, 10, 80, 120, '2')}</g></svg>`;

// the double-sided picture guide, one panel per step (step 3 is the animated flip, done in HTML)
function guideSteps(style, rot = false) {
  const rear = style === 'rear', P = rear ? printerRear : printerTray;
  // where finished pages come out, so the sheet can slide out from under the slot
  const slot = rear ? { x: 88, y: 164, w: 144 } : { x: 78, y: 104, w: 164 };
  const cover = `<rect x="${slot.x}" y="${slot.y}" width="${slot.w}" height="10" rx="5" fill="#5e2c3b"/>`;
  const out = `${sheet(slot.x + 22, slot.y + 6, slot.w - 44, rear ? 44 : 58, '1', 0)}${cover}
    <path d="M${slot.x + 8} ${slot.y + 26}l-26 8M${slot.x + 6} ${slot.y + 42}l-28 0M${slot.x + 12} ${slot.y + 58}l-24 10" stroke="var(--acc2)" stroke-width="4" stroke-linecap="round"/>`;
  const putBack = rear
    ? `${arrow(300, 110, 236, 36, -40)}`
    : `<rect x="50" y="150" width="220" height="56" rx="8" fill="#fff" stroke="var(--deep)" stroke-width="3"/>
       <path d="M64 166h192M64 174h192" stroke="var(--acc)" stroke-width="3.5"/>
       ${stack(126, 96, 68, 40, '')}${arrow(160, 64, 160, 92, 0)}
       <path d="M50 178h-22M270 178h22" stroke="var(--acc2)" stroke-width="4" stroke-linecap="round" stroke-dasharray="1 8"/>`;
  const steps = [
    ['Side 1 prints. Wait until it stops.', `<svg viewBox="0 0 320 220" class="art">${P(out)}</svg>`],
    ['Take the whole stack. Don\'t shuffle it.', `<svg viewBox="0 0 320 220" class="art"><g opacity=".25">${P('')}</g>
        ${stack(96, 36, 104, 136, '1')}${arrow(250, 196, 250, 24)}</svg>`],
    [`Flip it over like a book page. The ${FLOWER} stays at the top.`, 'FLIP'],
    ...(rot ? [[`Now turn it around: the ${FLOWER} goes to the bottom.`, 'SPIN']] : []),
    ['Put it back where the blank paper goes.', `<svg viewBox="0 0 320 220" class="art">${P('')}${putBack}</svg>`],
    ['Tap "Print the other side".', `<svg viewBox="0 0 320 220" class="art"><rect x="104" y="10" width="112" height="200" rx="20" fill="#fff" stroke="var(--deep)" stroke-width="4"/>
        <rect x="118" y="30" width="84" height="100" rx="8" fill="var(--soft)"/>${stack(140, 44, 40, 56, '2')}
        <rect x="118" y="146" width="84" height="34" rx="12" fill="var(--deep)"/><path d="M146 163h28" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
        <circle cx="178" cy="170" r="22" fill="none" stroke="var(--acc2)" stroke-width="4" opacity=".8"/><circle cx="178" cy="170" r="34" fill="none" stroke="var(--acc2)" stroke-width="3" opacity=".35"/></svg>`],
  ];
  return steps;
}
const LEGEND = {
  tray: [['1', 'Blank paper goes in the drawer at the front'], ['2', 'Printed pages come out here']],
  rear: [['1', 'Blank paper stands up at the back'], ['2', 'Printed pages come out the front']],
};
const legend = k => `<ul class="legend">${LEGEND[k].map(([n, t]) => `<li><span>${n}</span>${t}</li>`).join('')}</ul>`;

const flipAnim = () => `<div class="stage3d"><div class="stack"><i></i><i></i><div class="page"><div class="front"><b class="fl">${FLOWER}</b>1</div><div class="back"><b class="fl">${FLOWER}</b>2</div></div></div></div>`;
// turning the stack around: the flower (top of the page) swings from the top to the bottom
const spinAnim = () => `<div class="spinbox"><div class="spinsheet"><b class="fl">${FLOWER}</b>2</div><div class="spinarrow">${icon('turn')}</div></div>`;

// swipeable picture guide: big picture, big step number, one short line
function guide(style, from = 1, rot = false) {
  const steps = guideSteps(style, rot);
  return `<div class="guide"><div class="gtrack">${steps.map(([t, art], i) => `<figure class="gstep">
      <div class="gart">${art === 'FLIP' ? flipAnim() : art === 'SPIN' ? spinAnim() : art}</div>
      <figcaption><span class="gnum">${i + 1}</span>${t}</figcaption></figure>`).join('')}</div>
    <div class="gnav"><button class="gprev" aria-label="Back">‹</button>
      <div class="gdots">${steps.map((_, i) => `<i class="${i + 1 === from ? 'on' : ''}"></i>`).join('')}</div>
      <button class="gnext" aria-label="Next">›</button></div></div>`;
}
function wireGuide(root, from = 1) {
  root.querySelectorAll('.guide').forEach(g => {
    const track = g.querySelector('.gtrack'), dots = [...g.querySelectorAll('.gdots i')];
    const at = () => Math.round(track.scrollLeft / (track.clientWidth || 1));
    const show = i => { i = Math.max(0, Math.min(dots.length - 1, i)); track.scrollTo({ left: i * track.clientWidth, behavior: 'smooth' }); dots.forEach((d, k) => d.classList.toggle('on', k === i)); };
    track.addEventListener('scroll', () => { const i = at(); dots.forEach((d, k) => d.classList.toggle('on', k === i)); }, { passive: true });
    g.querySelector('.gprev').onclick = () => show(at() - 1);
    g.querySelector('.gnext').onclick = () => show(at() + 1);
    dots.forEach((d, k) => d.onclick = () => show(k));
    requestAnimationFrame(() => { track.scrollLeft = (from - 1) * track.clientWidth; });
  });
}

// ---------- state ----------
const S = {
  info: null, printer: '', opts: {}, calibrated: {}, style: {}, caps: {}, setupDone: {},
  roles: {}, labels: {},   // per printer: which driver option does what, and each choice in the driver's own words
  docs: [], photos: [], scans: [],
  doc: { sides: 'single', copies: 1, colour: 'mono', paper: '', quality: '', cover: 1, style: 1, palette: -1, whiteBg: false, fitPage: true, layout: '', skipBlank: false, extra: {} },
  pho: { layout: 'fill1', paper: '', glossy: false, colour: 'colour', copies: 1, media: '', quality: '' },
  scan: { dpi: 300, colour: true, device: '' },
  cp: { colour: false, copies: 1 },
  txt: { text: '', size: 13, heading: false, sides: 'single', copies: 1, key: '', files: [], paper: '', quality: '', colour: 'mono', media: '' },
  busy: false,
};

// ---------- api ----------
async function api(path, body, method) {
  const o = { method: method || (body ? 'POST' : 'GET'), headers: {} };
  if (body) { o.body = JSON.stringify(body); o.headers['Content-Type'] = 'application/json'; }
  const r = await fetch(path, o);
  let d = {};
  try { d = await r.json(); } catch (_) {}
  if (r.status === 401) { showPin(); throw new Error('locked'); }
  if (!r.ok) throw new Error(d.error || ('error ' + r.status));
  return d;
}

// guard: while one Print/Scan/Preview is running, extra taps are ignored (no double printing)
let inflight = false;
async function guard(fn) {
  if (inflight) return;
  inflight = true;
  try { await fn(); } finally { inflight = false; }
}

function toast(msg, bad) {
  const t = document.createElement('div');
  t.className = 'toast' + (bad ? ' bad' : '');
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), bad ? 6000 : 3200);
}
const oops = e => { if (e.message !== 'locked') toast(e.message, true); };

// ---------- the phone access switch (computer only) ----------
const tillText = iso => new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
function phonesLine(ph) {
  if (!ph) return '';
  return ph.state === 'on' ? 'Phones can connect.' : ph.state === 'until' ? `Phones can connect until ${tillText(ph.until)}.` : 'Phone access is off: phones can\'t connect.';
}
async function setPhones(v) {
  try { const r = await api('/api/settings', { phones: v }); S.info.phones = r.phones; } catch (e) { oops(e); }
}

// ---------- theme ----------
function mix(a, b, p) {
  const h = s => [1, 3, 5].map(i => parseInt(s.slice(i, i + 2), 16));
  const x = h(a), y = h(b);
  return '#' + x.map((v, i) => Math.round(v + (y[i] - v) * p / 100).toString(16).padStart(2, '0')).join('');
}
function applyTheme(i) {
  const p = S.info.palettes[i] || S.info.palettes[0];
  const r = document.documentElement.style;
  r.setProperty('--tint', p.tint); r.setProperty('--acc', p.acc); r.setProperty('--deep', p.deep);
  r.setProperty('--soft', mix(p.tint, '#f6f5f6', 88)); r.setProperty('--line', mix(p.acc, '#ececec', 88));
  $('meta[name=theme-color]').content = '#ffffff';
  const logo = [0, 72, 144, 216, 288].map(a => `<ellipse cy="-10" rx="7" ry="10" fill="${p.acc}" transform="rotate(${a})"/>`).join('') + `<circle r="5" fill="${p.deep}"/>`;
  $('#logo').innerHTML = logo; $('#logo2').innerHTML = logo;
}

// ---------- printer options ----------
async function loadOptions(p, force) {
  if (!p) return;
  if (!S.opts[p] || force) {
    const d = await api('/api/options?printer=' + encodeURIComponent(p));
    S.opts[p] = d.options || [];
    S.calibrated[p] = d.calibrated;
    S.style[p] = d.style || '';
    S.caps[p] = d.caps || {};
    S.setupDone[p] = !!d.setupDone;
    S.roles[p] = d.roles || {};
    S.labels[p] = d.labels || {};
  }
  const o = key(['PageSize']);
  if (o) {   // A4 unless you pick something else (only falls back to the printer's default if it has no A4)
    for (const c of [S.doc, S.pho, S.txt]) if (!c.paper || !o.values.includes(c.paper)) c.paper = a4Of(o);
  }
  const q = qualityOpt();
  if (q && !q.values.includes(S.doc.quality)) S.doc.quality = q.default;
  const c = colourOpt();
  if (c && !S.docColourSet) S.doc.colour = c.default === colourValue('mono') ? 'mono' : 'colour';
}
const opts = () => S.opts[S.printer] || [];
const key = names => opts().find(o => names.includes(o.key));
// which driver option does what comes from the computer, for any maker's driver (options.go); the short lists are
// only for an older computer that doesn't say
const role = () => S.roles[S.printer] || {};
const colourOpt = () => key(role().colour ? [role().colour] : ['ColorModel', 'print-color-mode', 'BRMonoColor']);
const qualityOpt = () => key(role().quality ? [role().quality] : ['cupsPrintQuality', 'print-quality', 'BRResolution']);
const mediaOpt = () => key(role().mediaType ? [role().mediaType] : ['MediaType', 'BRMediaType']);
// options the print screen has its own control for (the rest are under "More printer settings"); both-sides
// printing is the Sides switch, and the driver's own option would fight with it
const handled = () => ['PageSize', role().colour, role().quality, ...(role().duplex || []),
  'Duplex', 'sides', 'BRDuplex', 'EFDuplex', 'KMDuplex', 'JCLDuplex', 'OKDuplex', 'CNDuplex', 'XRDuplex'].filter(Boolean);

function colourValue(want) {
  const o = colourOpt(); if (!o) return null;
  const r = role();
  if (r.colour === o.key && (want === 'mono' ? r.monoOn : r.colourOn)) return want === 'mono' ? r.monoOn : r.colourOn;
  const re = want === 'mono' ? /mono|gray|grey|black/i : /colou?r|rgb|cmyk/i;
  return o.values.find(v => re.test(v)) || null;
}

// a choice in the driver's own words ("Other Photo Paper"), when the driver has them
function labelOf(v, optKey) {
  const l = S.labels[S.printer] || {};
  if (optKey && l[optKey] && l[optKey][v]) return l[optKey][v];
  for (const k in l) if (l[k][v]) return l[k][v];
  return '';
}
function paperName(v) {
  if (!v) return '';
  const own = labelOf(v, 'PageSize');
  if (own) return own.replace(/\s*\(?\bborderless\b\)?/i, '') + (/borderless/i.test(own) ? ' · no border' : '');
  const nice = {
    A4: 'A4', Letter: 'Letter (US)', Legal: 'Legal', A5: 'A5', A6: 'A6', B5: 'B5', B6: 'B6', Executive: 'Executive',
    PostC4x6: 'Photo 4×6 in (10×15 cm)', PhotoL: 'Photo L (3.5×5 in)', Photo2L: 'Photo 2L (5×7 in)', IndexC5x8: 'Index card 5×8 in',
    Postcard: 'Postcard', Hagaki: 'Hagaki postcard', EnvDL: 'Envelope DL', EnvC5: 'Envelope C5', Env10: 'Envelope #10', EnvMonarch: 'Envelope Monarch',
    Folio: 'Folio', IndiaLegal: 'India Legal', MexicanLegal: 'Mexican Legal', '4x6': 'Photo 4×6 in (10×15 cm)', '5x7': 'Photo 5×7 in', '3.5x5': 'Photo 3.5×5 in',
  };
  const borderless = /_B$|Borderless/i.test(v);
  let base = v.replace(/\.Borderless$/i, '').replace(/_[BS]$/, '').replace(/^Br(?=[A-Z0-9])/, '');
  return (nice[base] || base) + (borderless ? ' · no border' : '');
}
// a choice for the screen: the driver's own words, or its name made readable
const nicer = v => labelOf(v) || String(v).replace(/^Br(?=[A-Z0-9])/, '').replace(/([a-z])([A-Z])/g, '$1 $2');

// ---------- routing ----------
function go(r) { location.hash = '#/' + r; }
// the bug in v0.1: nothing listened for taps on the tiles. Now one listener handles every [data-go]
document.addEventListener('click', e => {
  const el = e.target.closest('[data-go]');
  if (el) { e.preventDefault(); go(el.dataset.go); }
});

// ---------- navigation: one list, drawn as a tab bar on phones and a sidebar on wider screens ----------
// the tab bar has room for five; the other screens light up the tab they belong to
const TABS = ['', 'docs', 'scan', 'printer', 'settings'];
const TAB_OF = { photos: 'docs', text: 'docs', copy: 'scan', again: '', addprinter: 'settings', advanced: 'settings', setup: '' };
const NAV_LABEL = { '': ['Home', 'home'], docs: ['Print', 'doc'], scan: ['Scan', 'scan'], printer: ['Printer', 'printer'], settings: ['Settings', 'gear'] };
function sideItems() {
  const items = [['', 'Home', 'home'], ['docs', 'Documents', 'doc'], ['photos', 'Photos', 'photo'], ['text', 'Text', 'text2'],
    ['scan', 'Scan', 'scan']];
  if (S.info && S.info.canScan) items.push(['copy', 'Copy', 'copy']);
  items.push(['again', 'Print again', 'redo'], ['printer', 'Printer care', 'printer']);
  if (S.info && S.info.local) items.push(['addprinter', 'Printers & drivers', 'plus']);
  items.push(['settings', 'Settings', 'gear']);
  if (S.info && S.info.local) items.push(['advanced', 'Advanced', 'info']);
  return items;
}
function drawNav(known) {
  const tab = known in TAB_OF ? TAB_OF[known] : known;
  $('#tabs').innerHTML = TABS.map(k => `<button data-go="${k}" class="${k === tab ? 'on' : ''}" aria-label="${NAV_LABEL[k][0]}"><svg viewBox="0 0 24 24">${I[NAV_LABEL[k][1]]}</svg>${NAV_LABEL[k][0]}</button>`).join('');
  $('#sidenav').innerHTML = sideItems().map(([k, label, ic]) => `<button data-go="${k}" class="${k === known ? 'on' : ''}"><svg viewBox="0 0 24 24">${I[ic] || I.doc}</svg><span>${label}</span></button>`).join('');
}

function route() {
  const r = location.hash.replace(/^#\/?/, '').split('/')[0];
  const known = r in routes ? r : '';
  document.body.classList.toggle('wizard-on', known === 'setup');
  $('#back').hidden = !known || known === 'setup';
  $('#brand').hidden = !!known;
  $('#ptitle').textContent = known ? F[known].t : 'Sakura Print';
  $('#pchip').hidden = !S.printer || ['settings', 'advanced', 'addprinter', 'setup'].includes(known);
  paintChip();
  // clear the previous screen's action bar BEFORE drawing the new screen (the new one adds its own)
  document.querySelectorAll('.actions').forEach(a => a.remove());
  document.body.classList.remove('has-actions');
  drawNav(known);
  (routes[r] || home)();
  window.scrollTo(0, 0);
  if (typeof refreshNotices === 'function') refreshNotices();
}
window.addEventListener('hashchange', route);
$('#back').onclick = () => go('');

let lastRoute = null;
function render(html) {
  const app = $('#app'), r = location.hash.replace(/^#\/?/, '').split('/')[0];
  app.innerHTML = html;
  if (r !== lastRoute) {   // only animate when you actually change screens, not on every little update
    app.style.setProperty('--dx', r === '' ? '-26px' : '26px');
    app.classList.remove('enter'); void app.offsetWidth; app.classList.add('enter');
    lastRoute = r;
  } else app.classList.remove('enter');
}
