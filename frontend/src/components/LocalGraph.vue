<template>
  <aside class="hidden lg:block w-1/3 sticky top-32 h-[420px] bg-white/40 dark:bg-black/20 backdrop-blur-3xl rounded-[2rem] border border-gray-200/50 dark:border-white/5 ml-12 overflow-hidden mt-16 shadow-[0_8px_30px_rgb(0,0,0,0.04)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.4)] relative group">
    <div class="px-7 py-5 border-b border-gray-100 dark:border-white/5 font-bold tracking-tight text-gray-900 dark:text-gray-100 text-sm flex items-center justify-between">
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-emerald-500"></div>
        Local Network
      </div>
      <span class="text-xs text-stone-400 font-mono font-normal">1-hop</span>
    </div>

    <!-- Floating Zoom Controls -->
    <div class="absolute bottom-4 right-4 z-10 opacity-70 hover:opacity-100 transition-opacity">
      <GraphZoomControls @zoom-in="zoomIn" @zoom-out="zoomOut" @fit="fitView" />
    </div>

    <div ref="networkContainer" class="w-full h-[360px]"></div>
  </aside>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import axios from 'axios';
import { Network } from 'vis-network';
import GraphZoomControls from './GraphZoomControls.vue';
import emitter from '../event-bus';

const props = defineProps<{
  articleId: string | number;
}>();

const router = useRouter();
const networkContainer = ref<HTMLElement | null>(null);

let network: Network | null = null;
let themeObserver: MutationObserver | null = null;

function getNumericId(): number {
  if (typeof props.articleId === 'number') return props.articleId;
  const clean = String(props.articleId || '')
    .replace(/^\/articles\//, '')
    .replace(/\.md$/, '');
  return parseInt(clean, 10);
}

import { useGraphZoom } from '../composables/useGraphZoom'
const { zoomIn, zoomOut, fitGraph: fitView } = useGraphZoom(() => network)



function getGraphColors() {
  const isDark = document.documentElement.classList.contains('dark');
  return {
    isDark,
    articleBg: '#10b981',
    articleBorder: isDark ? '#059669' : '#34d399',
    articleHighlight: isDark ? '#34d399' : '#10b981',
    articleText: isDark ? '#ffffff' : '#000000',
    mocBg: '#f59e0b',
    mocBorder: isDark ? '#d97706' : '#fbbf24',
    mocHighlight: '#fbbf24',
    tagBg: isDark ? '#1f2937' : '#f3f4f6',
    tagBorder: isDark ? '#374151' : '#e5e7eb',
    tagHighlight: isDark ? '#78716c' : '#a8a29e',
    tagText: isDark ? '#d1d5db' : '#4b5563',
    edgeColor: isDark ? '#333333' : '#e2e8f0',
    edgeHighlight: '#10b981',
  };
}

async function initLocalGraph() {
  const numericId = getNumericId();
  if (isNaN(numericId) || !numericId || !networkContainer.value) {
    return;
  }

  try {
    let graphNodes: Array<{ id: string; label: string; group: string }> = [];
    let graphEdges: Array<{ from: string; to: string }> = [];

    // Try fetching scoped local graph first
    try {
      const res = await axios.get<{ nodes: any[]; edges: any[] }>(`/api/graph/local/${numericId}`);
      if (res.data?.nodes?.length > 0) {
        graphNodes = res.data.nodes;
        graphEdges = res.data.edges || [];
      }
    } catch {
      // Fallback to global graph filtered locally
      const globalRes = await axios.get<{ nodes: any[]; edges: any[] }>('/api/graph');
      const allData = globalRes.data;
      const currentArticleNodeId = `article-${numericId}`;
      const connectedEdges = (allData.edges || []).filter(
        (e: any) => e.from === currentArticleNodeId || e.to === currentArticleNodeId
      );
      const connectedNodeIds = new Set<string>([currentArticleNodeId]);
      connectedEdges.forEach((e: any) => {
        connectedNodeIds.add(e.from);
        connectedNodeIds.add(e.to);
      });
      graphNodes = (allData.nodes || []).filter((n: any) => connectedNodeIds.has(n.id));
      graphEdges = connectedEdges;
    }

    if (!networkContainer.value) return;
    const colors = getGraphColors();

    const mocs = graphNodes.filter((n) => n.group === 'moc');
    const mocCount = Math.max(mocs.length, 1);
    const clusterRadius = 240;

    const mocPositions = new Map<string, { x: number; y: number; angle: number }>();
    mocs.forEach((moc, index) => {
      const angle = (index / mocCount) * 2 * Math.PI;
      mocPositions.set(moc.id, {
        x: Math.cos(angle) * clusterRadius,
        y: Math.sin(angle) * clusterRadius,
        angle,
      });
    });

    const articleMocMap = new Map<string, string>();
    graphEdges.forEach((e) => {
      if (mocPositions.has(e.from) && !mocPositions.has(e.to)) {
        articleMocMap.set(e.to, e.from);
      } else if (mocPositions.has(e.to) && !mocPositions.has(e.from)) {
        articleMocMap.set(e.from, e.to);
      }
    });

    const formattedNodes = graphNodes.map((node) => {
      const isMOC = node.group === 'moc';
      const isArticle = node.group === 'article';
      const isCurrent = node.id === `article-${numericId}`;
      const connections = graphEdges.filter((e) => e.from === node.id || e.to === node.id).length;

      let bg = colors.tagBg;
      let border = colors.tagBorder;
      let highlight = colors.tagHighlight;

      let nodeSize = 7;
      let fontSize = 9;

      if (isCurrent) {
        bg = '#10b981';
        border = '#34d399';
        highlight = '#34d399';
        nodeSize = Math.max(14, Math.min(14 + connections * 1.5, 24));
        fontSize = 12;
      } else if (isMOC) {
        bg = colors.mocBg;
        border = colors.mocBorder;
        highlight = colors.mocHighlight;
        nodeSize = Math.max(13, Math.min(13 + connections * 1.6, 26));
        fontSize = 11;
      } else if (isArticle) {
        bg = colors.articleBg;
        border = colors.articleBorder;
        highlight = colors.articleHighlight;
        nodeSize = Math.max(6, Math.min(6 + connections * 1.4, 20));
        fontSize = Math.max(9, Math.min(9 + Math.floor(connections * 0.3), 12));
      } else {
        nodeSize = Math.max(5, Math.min(5 + connections * 0.7, 14));
        fontSize = 9;
      }

      const showLocalLabel = isCurrent || isMOC || node.group === 'tag' || connections >= 4;
      const displayLabel = showLocalLabel ? (node.label.length > 24 ? node.label.slice(0, 22) + '…' : node.label) : undefined;
      const mass = isMOC ? 6.0 : (node.group === 'tag' ? 2.0 : 0.8);

      let initialX: number | undefined = undefined;
      let initialY: number | undefined = undefined;

      if (isMOC && mocPositions.has(node.id)) {
        const pos = mocPositions.get(node.id)!;
        initialX = pos.x;
        initialY = pos.y;
      } else if (articleMocMap.has(node.id)) {
        const parentPos = mocPositions.get(articleMocMap.get(node.id)!)!;
        const angle = parentPos.angle + (Math.random() - 0.5) * 1.4;
        const dist = 40 + Math.random() * 40;
        initialX = parentPos.x + Math.cos(angle) * dist;
        initialY = parentPos.y + Math.sin(angle) * dist;
      }

      return {
        id: node.id,
        label: displayLabel,
        title: `${node.label} (${connections} connection${connections === 1 ? '' : 's'})`,
        shape: isMOC ? 'hexagon' : (isArticle ? 'dot' : 'box'),
        size: nodeSize,
        mass: mass,
        x: initialX,
        y: initialY,
        margin: isArticle || isMOC ? 8 : 4,
        color: {
          background: bg,
          border: border,
          hover: {
            background: highlight,
            border: border,
          },
          highlight: {
            background: highlight,
            border: '#34d399',
          }
        },
        font: {
          color: isMOC ? (colors.isDark ? '#fef3c7' : '#92400e') : (colors.isDark ? '#e5e7eb' : '#374151'),
          size: fontSize,
          face: 'Inter, system-ui, sans-serif',
          strokeWidth: 3,
          strokeColor: colors.isDark ? '#0a0a0a' : '#f8f9fa',
          vadjust: isArticle || isMOC ? 2 : 0,
          bold: isCurrent || isMOC ? 'bold' : undefined,
        },
        borderWidth: isCurrent ? 2.5 : 1.5,
      };
    });

    const mocNodeIds = new Set(graphNodes.filter((n) => n.group === 'moc').map((n) => n.id));

    const formattedEdges = graphEdges.map((edge) => {
      const isMocEdge = mocNodeIds.has(edge.from) || mocNodeIds.has(edge.to);
      return {
        from: edge.from,
        to: edge.to,
        length: isMocEdge ? 65 : 220,
        springConstant: isMocEdge ? 0.045 : 0.015,
        color: isMocEdge
          ? { color: colors.isDark ? 'rgba(245, 158, 11, 0.25)' : 'rgba(217, 119, 6, 0.2)', highlight: '#f59e0b', hover: '#f59e0b' }
          : { color: colors.edgeColor, highlight: colors.edgeHighlight, hover: colors.edgeHighlight },
        width: 1,
        hoverWidth: 2,
        selectionWidth: 2,
        smooth: false,
      };
    });

    if (network) {
      network.destroy();
      network = null;
    }

    const options = {
      nodes: { borderWidthSelected: 2, widthConstraint: { maximum: 110 } },
      edges: { selectionWidth: 1.5 },
      layout: { improvedLayout: false },
      physics: {
        enabled: true,
        solver: 'repulsion',
        repulsion: {
          nodeDistance: 220,
          centralGravity: 0.003,
          springLength: 220,
          springConstant: 0.03,
          damping: 0.45,
        },
        stabilization: {
          enabled: true,
          iterations: 80,
          updateInterval: 25,
          fit: true,
        },
        minVelocity: 0.35,
        maxVelocity: 50,
      },
      interaction: {
        dragNodes: true,
        dragView: true,
        hover: true,
        hoverConnectedEdges: true,
        selectConnectedEdges: true,
        hideEdgesOnDrag: true,
        tooltipDelay: 100,
        zoomView: false,
      },
    };

    network = new Network(
      networkContainer.value,
      {
        nodes: formattedNodes as any,
        edges: formattedEdges as any,
      },
      options as any
    );

    let hasDraggedLocal = false;

    network.on('dragStart', () => {
      hasDraggedLocal = false;
    });

    network.on('dragging', () => {
      hasDraggedLocal = true;
    });

    network.on('dragEnd', () => {
      setTimeout(() => {
        hasDraggedLocal = false;
      }, 100);
    });

    network.on('click', (params: any) => {
      if (hasDraggedLocal) return;
      if (params.nodes && params.nodes.length > 0) {
        const selectedId = String(params.nodes[0]);
        if (selectedId.startsWith('article-')) {
          const articleId = selectedId.replace('article-', '');
          if (articleId !== String(numericId)) {
            router.push(`/articles/${articleId}`);
          }
        }
      }
    });
  } catch (err) {
    console.error('Failed to load local graph:', err);
  }
}

onMounted(() => {
  initLocalGraph();

  themeObserver = new MutationObserver(() => {
    initLocalGraph();
  });

  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  });

  emitter.on('article-added', () => {
    initLocalGraph();
  });
});

watch(
  () => props.articleId,
  () => {
    initLocalGraph();
  }
);

onUnmounted(() => {
  if (themeObserver) {
    themeObserver.disconnect();
    themeObserver = null;
  }
  if (network) {
    network.destroy();
    network = null;
  }
});
</script>
