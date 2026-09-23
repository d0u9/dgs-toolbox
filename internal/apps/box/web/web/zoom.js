// Wheel zoom and drag pan for a scan preview. The wheel zooms about the
// cursor, a held button drags the zoomed scan, a double-click fits it again,
// and a new image always starts fitted.

function zoomPan(image) {
  const frame = image.parentElement;
  const view = { scale: 1, x: 0, y: 0, drag: null };
  const minScale = 1;
  const maxScale = 8;

  function draw() {
    image.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.scale})`;
    frame.classList.toggle('zoomed', view.scale > 1);
  }

  function reset() {
    view.scale = 1;
    view.x = 0;
    view.y = 0;
    view.drag = null;
    frame.classList.remove('panning');
    draw();
  }

  frame.addEventListener('wheel', (event) => {
    if (!image.src || image.hidden) return;
    event.preventDefault();
    const next = Math.min(maxScale, Math.max(minScale, view.scale * Math.exp(-event.deltaY * 0.002)));
    if (next === minScale) {
      reset();
      return;
    }
    // The point under the cursor, in the image's untransformed box, stays put.
    const box = image.getBoundingClientRect();
    const px = event.clientX - (box.left - view.x);
    const py = event.clientY - (box.top - view.y);
    view.x = px - (px - view.x) * (next / view.scale);
    view.y = py - (py - view.y) * (next / view.scale);
    view.scale = next;
    draw();
  }, { passive: false });

  frame.addEventListener('pointerdown', (event) => {
    if (event.button !== 0 || view.scale === 1) return;
    event.preventDefault();
    view.drag = { id: event.pointerId, x: event.clientX - view.x, y: event.clientY - view.y };
    frame.setPointerCapture(event.pointerId);
    frame.classList.add('panning');
  });
  frame.addEventListener('pointermove', (event) => {
    if (!view.drag || view.drag.id !== event.pointerId) return;
    view.x = event.clientX - view.drag.x;
    view.y = event.clientY - view.drag.y;
    draw();
  });
  const release = (event) => {
    if (!view.drag || view.drag.id !== event.pointerId) return;
    view.drag = null;
    frame.classList.remove('panning');
  };
  frame.addEventListener('pointerup', release);
  frame.addEventListener('pointercancel', release);
  frame.addEventListener('dblclick', reset);
  image.addEventListener('load', reset);
  image.draggable = false;
  return { reset };
}

document.querySelectorAll('.sheet-frame > img, .detail-frame > img').forEach(zoomPan);
