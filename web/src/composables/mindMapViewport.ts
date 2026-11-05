import { nextTick, onUnmounted, ref } from 'vue'

type Pan = { x: number; y: number; left: number; top: number; moved: boolean }
type Pinch = { distance: number; zoom: number; contentX: number; contentY: number }

export function useMindMapViewport(canvasWidth: number) {
  const viewport = ref<HTMLDivElement>()
  const zoom = ref(1)
  const dragging = ref(false)
  let revision = 0
  let mouseDrag: (Pan & { id: number }) | undefined
  let touchPan: Pan | undefined
  let pinch: Pinch | undefined
  let suppressClickUntil = 0

  async function placeZoom(value: number, contentX: number, contentY: number, x: number, y: number) {
    const element = viewport.value
    if (!element) return
    const current = ++revision
    zoom.value = Math.max(.25, Math.min(1.75, value))
    await nextTick()
    if (revision !== current || viewport.value !== element) return
    element.scrollLeft = contentX * zoom.value - x
    element.scrollTop = contentY * zoom.value - y
  }
  function setZoom(value: number, x?: number, y?: number) {
    const element = viewport.value
    if (!element) return
    const anchorX = x ?? element.clientWidth / 2
    const anchorY = y ?? element.clientHeight / 2
    return placeZoom(value, (element.scrollLeft + anchorX) / zoom.value, (element.scrollTop + anchorY) / zoom.value, anchorX, anchorY)
  }
  async function resetView(fit = false) {
    const current = ++revision
    zoom.value = fit && viewport.value ? Math.min(1, viewport.value.clientWidth / canvasWidth) : 1
    await nextTick()
    if (revision === current) viewport.value?.scrollTo({ left: 0, top: 0 })
  }
  function wheel(event: WheelEvent) {
    if ((!event.ctrlKey && !event.metaKey) || !viewport.value) return
    event.preventDefault()
    const bounds = viewport.value.getBoundingClientRect()
    void setZoom(zoom.value + (event.deltaY < 0 ? .1 : -.1), event.clientX - bounds.left, event.clientY - bounds.top)
  }
  function panAt(x: number, y: number): Pan {
    return { x, y, left: viewport.value!.scrollLeft, top: viewport.value!.scrollTop, moved: false }
  }
  function movePan(pan: Pan, x: number, y: number) {
    if (!viewport.value) return
    viewport.value.scrollLeft = pan.left + pan.x - x
    viewport.value.scrollTop = pan.top + pan.y - y
  }
  function startDrag(event: PointerEvent) {
    if (!viewport.value || event.button !== 0 || event.pointerType !== 'mouse' || (event.target as Element).closest('a, button')) return
    mouseDrag = { ...panAt(event.clientX, event.clientY), id: event.pointerId }
    dragging.value = true
    viewport.value.setPointerCapture(event.pointerId)
    event.preventDefault()
  }
  function moveDrag(event: PointerEvent) {
    if (mouseDrag?.id === event.pointerId) movePan(mouseDrag, event.clientX, event.clientY)
  }
  function endDrag() { mouseDrag = undefined; dragging.value = false }

  // Handle both gestures on the canvas so a second finger can take over an
  // ongoing pan. touch-action:none keeps the browser from zooming the page.
  function touchesOnCanvas(event: TouchEvent) {
    return Array.from(event.touches).filter(touch => viewport.value?.contains(touch.target as Node))
  }
  function midpoint(first: Touch, second: Touch) {
    const bounds = viewport.value!.getBoundingClientRect()
    return { x: (first.clientX + second.clientX) / 2 - bounds.left, y: (first.clientY + second.clientY) / 2 - bounds.top }
  }
  function distance(first: Touch, second: Touch) { return Math.hypot(first.clientX - second.clientX, first.clientY - second.clientY) }
  function beginTouch(touches: Touch[]) {
    touchPan = undefined
    pinch = undefined
    dragging.value = false
    if (!viewport.value) return
    if (touches.length >= 2) {
      const center = midpoint(touches[0], touches[1])
      pinch = {
        distance: Math.max(1, distance(touches[0], touches[1])), zoom: zoom.value,
        contentX: (viewport.value.scrollLeft + center.x) / zoom.value,
        contentY: (viewport.value.scrollTop + center.y) / zoom.value,
      }
      suppressClickUntil = performance.now() + 500
    } else if (touches[0]) touchPan = panAt(touches[0].clientX, touches[0].clientY)
  }
  function startTouch(event: TouchEvent) {
    if (!touchPan && !pinch) suppressClickUntil = 0
    const touches = touchesOnCanvas(event)
    beginTouch(touches)
    if (touches.length >= 2) event.preventDefault()
  }
  function moveTouch(event: TouchEvent) {
    const touches = touchesOnCanvas(event)
    if (pinch && touches.length >= 2) {
      event.preventDefault()
      const center = midpoint(touches[0], touches[1])
      void placeZoom(pinch.zoom * distance(touches[0], touches[1]) / pinch.distance, pinch.contentX, pinch.contentY, center.x, center.y)
      suppressClickUntil = performance.now() + 500
    } else if (touchPan && touches[0]) {
      const touch = touches[0]
      touchPan.moved ||= Math.hypot(touch.clientX - touchPan.x, touch.clientY - touchPan.y) > 5
      if (!touchPan.moved) return
      event.preventDefault()
      dragging.value = true
      movePan(touchPan, touch.clientX, touch.clientY)
      suppressClickUntil = performance.now() + 500
    }
  }
  function endTouch(event: TouchEvent) {
    if (pinch || touchPan?.moved) suppressClickUntil = performance.now() + 500
    beginTouch(touchesOnCanvas(event))
  }
  function cancelTouch() {
    suppressClickUntil = performance.now() + 500
    touchPan = undefined
    pinch = undefined
    dragging.value = false
  }
  function guardClick(event: MouseEvent) {
    if (performance.now() >= suppressClickUntil) return
    event.preventDefault()
    event.stopPropagation()
  }
  onUnmounted(() => { revision++ })
  return { viewport, zoom, dragging, setZoom, resetView, wheel, startDrag, moveDrag, endDrag, startTouch, moveTouch, endTouch, cancelTouch, guardClick }
}
