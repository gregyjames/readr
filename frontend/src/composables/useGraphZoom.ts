import type { Network } from 'vis-network'

export function useGraphZoom(getNetwork: () => Network | null) {
  const zoomIn = () => {
    const net = getNetwork()
    if (!net) return
    const scale = net.getScale()
    net.moveTo({ scale: scale * 1.35, animation: { duration: 120, easingFunction: 'easeOutQuad' } })
  }

  const zoomOut = () => {
    const net = getNetwork()
    if (!net) return
    const scale = net.getScale()
    net.moveTo({ scale: scale / 1.35, animation: { duration: 120, easingFunction: 'easeOutQuad' } })
  }

  const fitGraph = () => {
    const net = getNetwork()
    if (!net) return
    net.fit({ animation: { duration: 150, easingFunction: 'easeOutQuad' } })
  }

  return { zoomIn, zoomOut, fitGraph }
}
