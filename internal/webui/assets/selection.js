// Rows are rebuilt when focus changes, so native dblclick can lose its target.
// Recognise the second plain click by row identity across those rebuilds.
export function rowDoubleClick() {
  let previous = null;
  const detect = (key, event) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.detail === 0) {
      previous = null;
      return false;
    }
    const double = previous?.key === key && event.timeStamp - previous.time <= 500
      && event.timeStamp >= previous.time
      && Math.hypot(event.clientX - previous.x, event.clientY - previous.y) <= 4;
    previous = double ? null : { key, time: event.timeStamp, x: event.clientX, y: event.clientY };
    return double;
  };
  detect.reset = () => { previous = null; };
  return detect;
}
