import type { Network } from 'vis-network'

export function useGraphZoom(getNetwork: () => Network | null) {
  const zoomIn = () => {
    const net = getNetwork()
    if (!net) return
    const scale = net.getScale()
    net.moveTo({ scale: scale * 1.3, animation: { duration: 200, easingFunction: 'easeInOutQuad' } })
  }

  const zoomOut = () => {
    const net = getNetwork()
    if (!net) return
    const scale = net.getScale()
    net.moveTo({ scale: scale / 1.3, animation: { duration: 200, easingFunction: 'easeInOutQuad' } })
  }

  const fitGraph = () => {
    const net = getNetwork()
    if (!net) return
    net.fit({ animation: { duration: 200, easingFunction: 'easeInOutQuad' } })
  }

  return { zoomIn, zoomOut, fitGraph }
}
