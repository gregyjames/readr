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

    const formattedNodes = graphNodes.map((node) => {
      const isArticle = node.group === 'article';
      const isCurrent = node.id === `article-${numericId}`;

      return {
        id: node.id,
        label: isArticle ? undefined : node.label,
        title: node.label,
        shape: isArticle ? 'dot' : 'box',
        size: isCurrent ? 14 : 10,
        margin: isArticle ? 10 : 8,
        color: {
          background: isCurrent ? '#059669' : (isArticle ? colors.articleBg : colors.tagBg),
          border: isCurrent ? '#34d399' : (isArticle ? colors.articleBorder : colors.tagBorder),
          hover: {
            background: isArticle ? colors.articleHighlight : colors.tagHighlight,
            border: isArticle ? colors.articleBorder : colors.tagBorder,
          },
        },
        font: {
          color: colors.isDark ? '#fff' : '#000',
          size: isCurrent ? 12 : 10,
          face: 'Outfit, sans-serif',
        },
        borderWidth: isCurrent ? 3 : 2,
      };
    });

    const formattedEdges = graphEdges.map((edge) => ({
      from: edge.from,
      to: edge.to,
      color: {
        color: colors.edgeColor,
        highlight: colors.edgeHighlight,
      },
      width: 1.5,
      smooth: { enabled: true, type: 'continuous', roundness: 0.5 },
    }));

    if (network) {
      network.destroy();
      network = null;
    }

    const options = {
      nodes: { borderWidthSelected: 2, widthConstraint: { maximum: 120 } },
      edges: { selectionWidth: 2 },
      physics: {
        barnesHut: {
          gravitationalConstant: -2000,
          centralGravity: 0.3,
          springLength: 90,
          damping: 0.2,
        },
      },
      interaction: {
        hover: true,
        tooltipDelay: 100,
        zoomView: false, // Disabled mouse wheel zoom
        dragView: true,
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

    network.on('click', (params: any) => {
      if (params.nodes && params.nodes.length > 0) {
        const selectedId = String(params.nodes[0]);
        if (selectedId.startsWith('article-')) {
          const targetId = selectedId.replace('article-', '');
          if (targetId !== String(numericId)) {
            router.push(`/articles/${targetId}`);
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
