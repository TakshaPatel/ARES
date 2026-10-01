import type { StyleSpecification } from 'maplibre-gl'

const ENV_KEY = import.meta.env.VITE_MAPTILER_KEY as string | undefined

export const MAPTILER_KEY = ENV_KEY ?? 'cb1_440v_1_1272839a39afc820f8998538'

export const GLYPHS = 'https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf'

export const SATELLITE_STYLE = MAPTILER_KEY
  ? `https://api.maptiler.com/maps/satellite/style.json?key=${MAPTILER_KEY}`
  : null

export const FALLBACK_STYLE: StyleSpecification = {
  version: 8,
  glyphs: GLYPHS,
  sources: {
    base: {
      type: 'raster',
      tiles: [
        'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
      ],
      tileSize: 256,
      maxzoom: 18,
      attribution: 'Imagery &copy; Esri, Maxar, Earthstar Geographics',
    },
  },
  layers: [{ id: 'base', type: 'raster', source: 'base' }],
}

export const CENTER: [number, number] = [-74.48793, 40.47456]
export const ZOOM = 11.6
