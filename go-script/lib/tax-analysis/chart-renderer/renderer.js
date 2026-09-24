import { Resvg, initWasm } from "@resvg/resvg-wasm";
import opentype from "opentype.js";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  CANVAS,
  CHART_COLORS,
  COLORS,
  DONUT_LAYOUT,
  EMPTY_LAYOUT,
  FONT,
  GROUPED_COLUMN_LAYOUT,
  HORIZONTAL_BAR_LAYOUT,
  LABEL_LAYOUT,
  LEGEND_LAYOUT,
  LINE_LAYOUT,
  NETWORK_LAYOUT,
  NUMBER_UNITS,
  PANEL_LAYOUT,
  PIE_LAYOUT,
  QUADRANT_LAYOUT,
  RADAR_LAYOUT,
  STACKED_BAR_LAYOUT,
  STROKE,
  TEXT,
  TIMELINE_LAYOUT,
  TITLE_LAYOUT,
} from "./renderer.constants.js";

const wasmPath = fileURLToPath(import.meta.resolve("@resvg/resvg-wasm/index_bg.wasm"));

/**
 * Parse renderer command-line arguments.
 * @param {string[]} argv Complete process argument list.
 * @returns {Record<string, string>} Recognized argument values keyed without leading dashes.
 */
function parseArgs(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index];
    if (value === "--regular-font" || value === "--bold-font" || value === "--output-file" || value === "--svg-output-file") {
      result[value.slice(2)] = argv[index + 1] ?? "";
      index += 1;
    }
  }
  return result;
}

/**
 * Escape a value for safe inclusion in SVG XML.
 * @param {unknown} value Raw value.
 * @returns {string} XML-escaped text.
 */
function escapeXML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

/**
 * Convert an input to a finite number for drawing calculations.
 * @param {unknown} value Raw numeric value.
 * @returns {number} The finite numeric value, or zero when invalid.
 */
function finiteNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : 0;
}

/**
 * Measure rendered text width with the packaged font.
 * @param {opentype.Font} font Parsed OpenType font.
 * @param {unknown} value Text value.
 * @param {number} size Font size in pixels.
 * @returns {number} Rounded advance width in pixels.
 */
function measure(font, value, size) {
  return Math.ceil(font.getAdvanceWidth(String(value ?? ""), size, { kerning: true }));
}

/**
 * Fit text into a fixed width and append an ellipsis when necessary.
 * @param {opentype.Font} font Parsed OpenType font.
 * @param {unknown} value Text value.
 * @param {number} size Font size in pixels.
 * @param {number} maximum Maximum width in pixels.
 * @returns {string} Original or shortened text.
 */
function fitText(font, value, size, maximum) {
  const text = String(value ?? "");
  if (measure(font, text, size) <= maximum) return text;
  const runes = Array.from(text);
  while (runes.length > 0) {
    const candidate = `${runes.join("")}…`;
    if (measure(font, candidate, size) <= maximum) return candidate;
    runes.pop();
  }
  return "";
}

/**
 * Wrap text without discarding any characters.
 * @param {opentype.Font} font Parsed OpenType font.
 * @param {unknown} value Text value.
 * @param {number} size Font size in pixels.
 * @param {number} maximum Maximum width of one line in pixels.
 * @returns {string[]} Non-empty wrapped lines.
 */
function wrapText(font, value, size, maximum) {
  const text = String(value ?? "");
  if (text === "") return [""];
  const lines = [];
  let line = "";
  for (const rune of Array.from(text)) {
    const candidate = line + rune;
    if (line !== "" && measure(font, candidate, size) > maximum) {
      lines.push(line);
      line = rune;
    } else {
      line = candidate;
    }
  }
  if (line !== "" || lines.length === 0) lines.push(line);
  return lines;
}

/**
 * Fit complete text into a bounded number of lines by reducing its font size.
 * @param {opentype.Font} font Parsed OpenType font.
 * @param {unknown} value Text value.
 * @param {number} preferredSize Preferred font size in pixels.
 * @param {number} minimumSize Smallest permitted font size in pixels.
 * @param {number} maximumWidth Maximum line width in pixels.
 * @param {number} maximumLines Target maximum line count.
 * @returns {{lines:string[],size:number}} Complete text and selected size.
 */
function fitWrappedText(font, value, preferredSize, minimumSize, maximumWidth, maximumLines) {
  for (let size = preferredSize; size >= minimumSize; size -= 1) {
    const lines = wrapText(font, value, size, maximumWidth);
    if (lines.length <= maximumLines) return { lines, size };
  }
  return { lines: wrapText(font, value, minimumSize, maximumWidth), size: minimumSize };
}

/**
 * Render complete wrapped text vertically centered around one chart coordinate.
 * @param {string[]} lines Wrapped text lines.
 * @param {number} x Shared x coordinate.
 * @param {number} centerY Vertical center coordinate.
 * @param {number} size Font size in pixels.
 * @param {number} lineHeight Line height in pixels.
 * @param {string} attributes SVG text attributes.
 * @returns {string} One SVG text node containing positioned tspans.
 */
function multilineText(lines, x, centerY, size, lineHeight, attributes) {
  const firstBaseline = centerY - (lines.length - 1) * lineHeight / 2 + size * 0.35;
  const spans = lines.map((line, index) => `<tspan x="${x}" y="${firstBaseline + index * lineHeight}">${escapeXML(line)}</tspan>`).join("");
  return `<text ${attributes}>${spans}</text>`;
}

/**
 * Calculate a rounded chart-axis maximum.
 * @param {number} value Largest visible data value.
 * @returns {number} Positive rounded axis maximum.
 */
function niceMaximum(value) {
  if (!(value > 0)) return 1;
  const exponent = Math.floor(Math.log10(value));
  const magnitude = 10 ** exponent;
  const normalized = value / magnitude;
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return step * magnitude;
}

/**
 * Calculate rounded bounds for an axis that may contain negative values.
 * @param {number[]} values Available numeric values.
 * @returns {{minimum:number,maximum:number}} Rounded bounds containing zero.
 */
function niceAxisBounds(values) {
  if (!Array.isArray(values) || values.length === 0) return { minimum: 0, maximum: 1 };
  const rawMinimum = Math.min(0, ...values.map(finiteNumber));
  const rawMaximum = Math.max(0, ...values.map(finiteNumber));
  const minimum = rawMinimum < 0 ? -niceMaximum(Math.abs(rawMinimum)) : 0;
  let maximum = rawMaximum > 0 ? niceMaximum(rawMaximum) : 0;
  if (minimum === maximum) maximum = 1;
  return { minimum, maximum };
}

/**
 * Convert an axis value to a vertical plot coordinate.
 * @param {number} value Numeric source value.
 * @param {{minimum:number,maximum:number}} bounds Rounded axis bounds.
 * @param {number} top Plot top.
 * @param {number} bottom Plot bottom.
 * @returns {number} SVG y coordinate inside the plot.
 */
function axisY(value, bounds, top, bottom) {
  return bottom - (finiteNumber(value) - bounds.minimum) / (bounds.maximum - bounds.minimum) * (bottom - top);
}

/**
 * Format a chart number with Chinese display units.
 * @param {unknown} value Numeric value.
 * @returns {string} Compact human-readable number.
 */
function compactNumber(value) {
  const number = finiteNumber(value);
  if (Math.abs(number) >= NUMBER_UNITS.hundredMillion) return `${(number / NUMBER_UNITS.hundredMillion).toFixed(2)}亿`;
  if (Math.abs(number) >= NUMBER_UNITS.tenThousand) return `${(number / NUMBER_UNITS.tenThousand).toFixed(2)}万`;
  if (Number.isInteger(number)) return String(number);
  return number.toFixed(2);
}

/**
 * Wrap chart content in the shared component-library SVG frame.
 * @param {string} title Chart title.
 * @param {string} body SVG body markup.
 * @param {opentype.Font} titleFont Parsed bold font used for title measurement.
 * @param {{x?: number, y?: number, anchor?: string, maximumWidth?: number, panelHeader?: boolean, hideTitle?: boolean}} [titleOptions] Optional title placement and Figma panel treatment.
 * @returns {string} Complete SVG document.
 */
function svgShell(title, body, titleFont, titleOptions = {}) {
  const titleX = titleOptions.x ?? TITLE_LAYOUT.x;
  const titleY = titleOptions.y ?? TITLE_LAYOUT.y;
  const titleAnchor = titleOptions.anchor ?? "start";
  const titleMaximumWidth = titleOptions.maximumWidth ?? TITLE_LAYOUT.maximumWidth;
  const safeTitle = fitText(titleFont, title, FONT.titleSize, titleMaximumWidth);
  const panelHeader = titleOptions.panelHeader
    ? `<rect x="${CANVAS.frameInset}" y="${CANVAS.frameInset}" width="${CANVAS.frameWidth}" height="${PANEL_LAYOUT.headerHeight}" fill="${COLORS.panelHeader}"/>
  <line x1="${CANVAS.frameInset}" y1="${PANEL_LAYOUT.shadowY}" x2="${CANVAS.width - CANVAS.frameInset}" y2="${PANEL_LAYOUT.shadowY}" stroke="${COLORS.panelShadow}"/>
  <text class="title" data-role="panel-title" x="${PANEL_LAYOUT.titleX}" y="${PANEL_LAYOUT.titleY}">${escapeXML(fitText(titleFont, title, FONT.titleSize, PANEL_LAYOUT.titleMaximumWidth))}</text>`
    : "";
  const titleMarkup = titleOptions.hideTitle
    ? ""
    : `<text class="title" data-role="chart-title" x="${titleX}" y="${titleY}" text-anchor="${escapeXML(titleAnchor)}">${escapeXML(safeTitle)}</text>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${CANVAS.width}" height="${CANVAS.height}" viewBox="0 0 ${CANVAS.width} ${CANVAS.height}">
  <rect x="${CANVAS.frameInset}" y="${CANVAS.frameInset}" width="${CANVAS.frameWidth}" height="${CANVAS.frameHeight}" fill="${COLORS.canvas}" stroke="${COLORS.frameBorder}"/>
  <style>
    text { font-family: '${FONT.family}'; font-size: ${FONT.legendSize}px; fill: ${COLORS.bodyText}; }
    .title { font-size: ${FONT.titleSize}px; font-weight: 700; fill: ${COLORS.headingText}; }
    .legend { font-size: ${FONT.legendSize}px; fill: ${COLORS.headingText}; }
    .axis { font-size: ${FONT.axisSize}px; fill: ${COLORS.bodyText}; }
    .grid { stroke: ${COLORS.grid}; stroke-width: ${STROKE.gridWidth}; stroke-dasharray: ${STROKE.gridDash}; }
    .axis-line { stroke: ${COLORS.axisLine}; stroke-width: ${STROKE.axisWidth}; }
  </style>
  ${panelHeader}
  ${titleMarkup}
  ${body}
</svg>`;
}

/**
 * Lay out legend entries inside the reserved right-side region.
 * @param {Array<{name: string}>} series Chart series.
 * @param {opentype.Font} font Parsed regular font.
 * @param {number} top Top offset in pixels.
 * @returns {{entries: Array<object>, bottom: number}} Positioned legend entries and occupied bottom edge.
 */
function legendLayout(series, font, top = LEGEND_LAYOUT.top) {
  const left = LEGEND_LAYOUT.left;
  const right = CANVAS.width - LEGEND_LAYOUT.rightPadding;
  const marker = LEGEND_LAYOUT.markerSize;
  const markerGap = LEGEND_LAYOUT.markerGap;
  const itemGap = LEGEND_LAYOUT.itemGap;
  const rowHeight = LEGEND_LAYOUT.rowHeight;
  const items = series.map((item, index) => {
    const text = fitText(font, item.name, FONT.legendSize, right - left - marker - markerGap);
    const textWidth = measure(font, text, FONT.legendSize);
    return { index, text, textWidth, width: marker + markerGap + textWidth };
  });
  const rows = [];
  let current = [];
  let currentWidth = 0;
  for (const item of items) {
    const candidate = current.length === 0 ? item.width : currentWidth + itemGap + item.width;
    if (current.length > 0 && candidate > right - left) {
      rows.push({ items: current, width: currentWidth });
      current = [];
      currentWidth = 0;
    }
    if (current.length > 0) currentWidth += itemGap;
    current.push(item);
    currentWidth += item.width;
  }
  if (current.length > 0) rows.push({ items: current, width: currentWidth });

  const entries = [];
  rows.forEach((row, rowIndex) => {
    let x = right - row.width;
    const baseline = top + LEGEND_LAYOUT.baselineOffset + rowIndex * rowHeight;
    for (const item of row.items) {
      entries.push({ ...item, x, baseline, rowIndex });
      x += item.width + itemGap;
    }
  });
  return { entries, bottom: rows.length === 0 ? top : top + LEGEND_LAYOUT.bottomOffset + (rows.length - 1) * rowHeight };
}

/**
 * Render a responsive, multi-row SVG legend.
 * @param {Array<{name: string, color?: string}>} series Chart series.
 * @param {opentype.Font} font Parsed regular font.
 * @returns {{svg: string, bottom: number}} Legend SVG and occupied bottom edge.
 */
function renderLegend(series, font) {
  const layout = legendLayout(series, font);
  const svg = layout.entries.map((entry) => {
    const color = series[entry.index].color || CHART_COLORS[entry.index % CHART_COLORS.length];
    return `<g data-role="legend-item" data-row="${entry.rowIndex}" data-left="${entry.x}" data-right="${entry.x + entry.width}"><rect x="${entry.x}" y="${entry.baseline - LEGEND_LAYOUT.markerSize}" width="${LEGEND_LAYOUT.markerSize}" height="${LEGEND_LAYOUT.markerSize}" rx="2" fill="${escapeXML(color)}"/><text class="legend" x="${entry.x + LEGEND_LAYOUT.markerSize + LEGEND_LAYOUT.markerGap}" y="${entry.baseline}">${escapeXML(entry.text)}</text></g>`;
  }).join("");
  return { svg, bottom: layout.bottom };
}

/**
 * Render the Figma module-risk legend from the left edge below its centered title.
 * @param {Array<{name: string, color?: string}>} series Chart series.
 * @param {opentype.Font} font Parsed regular font.
 * @returns {{svg: string, bottom: number}} Legend SVG and occupied bottom edge.
 */
function renderStackedBarLegend(series, font) {
  let x = STACKED_BAR_LAYOUT.legendLeft;
  const baseline = STACKED_BAR_LAYOUT.legendTop + LEGEND_LAYOUT.baselineOffset;
  const items = [];
  series.forEach((item, index) => {
    const text = fitText(font, item.name, FONT.legendSize, 160);
    const textWidth = measure(font, text, FONT.legendSize);
    const color = item.color || CHART_COLORS[index % CHART_COLORS.length];
    items.push(`<g data-role="legend-item"><rect x="${x}" y="${baseline - LEGEND_LAYOUT.markerSize}" width="${LEGEND_LAYOUT.markerSize}" height="${LEGEND_LAYOUT.markerSize}" rx="2" fill="${escapeXML(color)}"/><text class="legend" x="${x + LEGEND_LAYOUT.markerSize + LEGEND_LAYOUT.markerGap}" y="${baseline}">${escapeXML(text)}</text></g>`);
    x += LEGEND_LAYOUT.markerSize + LEGEND_LAYOUT.markerGap + textWidth + LEGEND_LAYOUT.itemGap;
  });
  return { svg: items.join(""), bottom: baseline + LEGEND_LAYOUT.bottomOffset };
}

/**
 * Calculate the x-coordinate for a category.
 * @param {number} index Category index.
 * @param {number} count Category count.
 * @param {number} left Plot left edge.
 * @param {number} right Plot right edge.
 * @returns {number} Category center x-coordinate.
 */
function categoryX(index, count, left, right) {
  if (count <= 1) return (left + right) / 2;
  return left + index * (right - left) / (count - 1);
}

/**
 * Check whether a series point represents available source data.
 * @param {{available?: boolean[]}} series Chart series.
 * @param {number} index Point index.
 * @returns {boolean} Whether the point may be drawn.
 */
function pointAvailable(series, index) {
  return !Array.isArray(series.available) || series.available.length === 0 || series.available[index] !== false;
}

/**
 * Resolve one categorical slice color from the report model.
 * @param {object} chart Normalized chart model.
 * @param {number} index Zero-based category index.
 * @returns {string} Model-provided or fallback color.
 */
function categoricalColor(chart, index) {
  const colors = chart.series?.[0]?.colors;
  if (Array.isArray(colors) && colors[index]) return String(colors[index]);
  return CHART_COLORS[index % CHART_COLORS.length];
}

/**
 * Determine whether a chart contains at least one explicitly available datum.
 * @param {object} chart Normalized chart model.
 * @returns {boolean} True when the selected renderer has drawable source data.
 */
function chartHasData(chart) {
  if (chart.kind === "network") return (chart.labels?.length ?? 0) > 1;
  if (chart.kind === "timeline") return (chart.labels?.length ?? 0) > 0 && (chart.series?.length ?? 0) > 0;
  if (chart.kind === "quadrant" || chart.kind === "priority_matrix") {
    return (chart.points ?? []).some((point) => point.available !== false && Number.isFinite(Number(point.x)) && Number.isFinite(Number(point.y)));
  }
  return (chart.series ?? []).some((series) => (series.values ?? []).some((value, index) => pointAvailable(series, index) && Number.isFinite(Number(value))));
}

/**
 * Render the Figma-style neutral empty state used by required chart sections.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function emptyChart(chart, fonts) {
  const message = fitText(fonts.regular, chart.emptyMessage || TEXT.emptyBody, FONT.emptyBodySize, EMPTY_LAYOUT.maximumBodyWidth);
  const body = `<rect x="${EMPTY_LAYOUT.left}" y="${EMPTY_LAYOUT.top}" width="${EMPTY_LAYOUT.right - EMPTY_LAYOUT.left}" height="${EMPTY_LAYOUT.bottom - EMPTY_LAYOUT.top}" rx="10" fill="${COLORS.emptyBackground}" stroke="${COLORS.emptyBorder}" stroke-dasharray="${STROKE.emptyDash}"/>
  <circle cx="${EMPTY_LAYOUT.iconCenterX}" cy="${EMPTY_LAYOUT.iconCenterY}" r="${EMPTY_LAYOUT.iconRadius}" fill="${COLORS.paleBlue}"/>
  <path d="M${EMPTY_LAYOUT.iconCenterX - 15} ${EMPTY_LAYOUT.iconCenterY + 8} L${EMPTY_LAYOUT.iconCenterX - 3} ${EMPTY_LAYOUT.iconCenterY - 7} L${EMPTY_LAYOUT.iconCenterX + 8} ${EMPTY_LAYOUT.iconCenterY + 2} L${EMPTY_LAYOUT.iconCenterX + 17} ${EMPTY_LAYOUT.iconCenterY - 13}" fill="none" stroke="${COLORS.primary}" stroke-width="${STROKE.emptyIconWidth}" stroke-linecap="round" stroke-linejoin="round"/>
  <text x="${EMPTY_LAYOUT.iconCenterX}" y="${EMPTY_LAYOUT.titleY}" text-anchor="middle" style="font-size:${FONT.emptyTitleSize}px;font-weight:700;fill:${COLORS.headingText}">${escapeXML(chart.emptyTitle || TEXT.emptyTitle)}</text>
  <text x="${EMPTY_LAYOUT.iconCenterX}" y="${EMPTY_LAYOUT.bodyY}" text-anchor="middle" style="font-size:${FONT.emptyBodySize}px;fill:${COLORS.mutedText}">${escapeXML(message)}</text>`;
  return svgShell(chart.title, body, fonts.bold);
}

/**
 * Format an axis tick without changing the model's numeric scale.
 * @param {number} value Tick value, already expressed in the declared unit.
 * @param {string} unit Axis unit.
 * @returns {string} Complete tick label.
 */
function axisTick(value, unit) {
  return compactNumber(value) + (unit === "%" ? "%" : "");
}

/**
 * Measure Cartesian axes and retain every category label without ellipses.
 * Glyph bounds are rotated before checking neighbouring labels and the frame.
 * @param {string[]} labels Original categories or periods.
 * @param {opentype.Font} font Packaged regular font.
 * @param {object} base Default plot rectangle and grid settings.
 * @param {string[]} leftTicks Formatted left-axis labels.
 * @param {string[]} rightTicks Formatted right-axis labels, empty for one axis.
 * @param {boolean} centered Whether points lie at column centres.
 * @returns {object} Plot bounds, complete SVG labels and selected angle.
 */
function cartesianLayout(labels, font, base, leftTicks, rightTicks = [], centered = false) {
  const padding = CANVAS.frameInset + LABEL_LAYOUT.framePadding;
  const leftWidth = Math.max(0, ...leftTicks.map((label) => measure(font, label, FONT.axisSize)));
  const rightWidth = Math.max(0, ...rightTicks.map((label) => measure(font, label, FONT.axisSize)));
  const axisLeft = Math.max(base.left, padding + leftWidth + LABEL_LAYOUT.tickGap);
  const axisRight = Math.min(base.right, CANVAS.width - padding - (rightTicks.length ? rightWidth + LABEL_LAYOUT.tickGap : 0));
  const display = labels.map((label) => label.replace(/^(\d{4})年?第?([1-4])季度$/, "$1Q$2"));
  let selected;
  for (const angle of LABEL_LAYOUT.angles) {
    const radians = angle * Math.PI / 180;
    const boxes = display.map((label) => {
      const width = measure(font, label, FONT.axisSize);
      const b = font.getPath(label, -width / 2, 0, FONT.axisSize).getBoundingBox();
      const points = [[b.x1, b.y1], [b.x1, b.y2], [b.x2, b.y1], [b.x2, b.y2]]
        .map(([x, y]) => [x * Math.cos(radians) - y * Math.sin(radians), x * Math.sin(radians) + y * Math.cos(radians)]);
      return { left: Math.min(...points.map((p) => p[0])), right: Math.max(...points.map((p) => p[0])),
        top: Math.min(...points.map((p) => p[1])), bottom: Math.max(...points.map((p) => p[1])) };
    });
    const left = Math.max(axisLeft, padding - (boxes[0]?.left ?? 0));
    const right = Math.min(axisRight, CANVAS.width - padding - (boxes.at(-1)?.right ?? 0));
    const xs = labels.map((_, index) => centered ? left + (index + 0.5) * (right - left) / labels.length : categoryX(index, labels.length, left, right));
    const topOffset = Math.min(0, ...boxes.map((box) => box.top));
    const bottomOffset = Math.max(0, ...boxes.map((box) => box.bottom));
    const bottom = Math.min(base.bottom, CANVAS.height - LABEL_LAYOUT.bottomPadding - LABEL_LAYOUT.axisGap - (bottomOffset - topOffset));
    const y = bottom + LABEL_LAYOUT.axisGap - topOffset;
    selected = { left, right, bottom, angle, boxes, xs, y };
    if (boxes.every((box, index) => index === 0 || xs[index - 1] + boxes[index - 1].right + LABEL_LAYOUT.collisionGap <= xs[index] + box.left)) break;
  }
  const { boxes, xs, y, angle } = selected;
  selected.svg = display.map((label, index) => {
    const b = boxes[index];
    return `<text class="axis" data-role="category-label" data-index="${index}" data-left="${xs[index] + b.left}" data-right="${xs[index] + b.right}" data-top="${y + b.top}" data-bottom="${y + b.bottom}" transform="translate(${xs[index]} ${y}) rotate(${angle})" text-anchor="middle">${escapeXML(label)}</text>`;
  }).join("");
  return selected;
}

/**
 * Render a line-chart SVG and preserve gaps for unavailable periods.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function lineChart(chart, fonts) {
  const series = chart.series ?? [];
  const labels = (chart.labels ?? []).map(String);
  const legend = renderLegend(series, fonts.regular);
  const top = Math.max(LINE_LAYOUT.minimumTop, legend.bottom + LINE_LAYOUT.legendGap);
  const bounds = niceAxisBounds(series.flatMap((item) => (item.values ?? []).flatMap((value, index) => pointAvailable(item, index) ? [finiteNumber(value)] : [])));
  const ticks = Array.from({ length: LINE_LAYOUT.gridSteps + 1 }, (_, index) => axisTick(bounds.maximum - index * (bounds.maximum - bounds.minimum) / LINE_LAYOUT.gridSteps, chart.unit));
  const layout = cartesianLayout(labels, fonts.regular, LINE_LAYOUT, ticks);
  const { left, right, bottom } = layout;
  const plotHeight = Math.max(1, bottom - top);
  const parts = [legend.svg];
  for (let index = 0; index <= LINE_LAYOUT.gridSteps; index += 1) {
    const y = top + index * plotHeight / LINE_LAYOUT.gridSteps;
    parts.push(`<line class="grid" x1="${left}" y1="${y}" x2="${right}" y2="${y}"/>`);
    parts.push(`<text class="axis" data-role="left-axis-tick" x="${left - LABEL_LAYOUT.tickGap}" y="${y + 5}" text-anchor="end">${escapeXML(ticks[index])}</text>`);
  }
  const zeroY = axisY(0, bounds, top, bottom);
  parts.push(`<line class="axis-line" x1="${left}" y1="${top}" x2="${left}" y2="${bottom}"/><line class="axis-line" x1="${left}" y1="${zeroY}" x2="${right}" y2="${zeroY}"/>`);
  series.forEach((item, seriesIndex) => {
    const color = item.color || CHART_COLORS[seriesIndex % CHART_COLORS.length];
    const pointCount = Math.min(labels.length, item.values?.length ?? 0);
    const segments = [];
    let points = [];
    for (let index = 0; index < pointCount; index += 1) {
      if (!pointAvailable(item, index)) {
        if (points.length > 0) segments.push(points);
        points = [];
        continue;
      }
      const x = categoryX(index, labels.length, left, right);
      const y = axisY(item.values[index], bounds, top, bottom);
      points.push([x, y]);
    }
    if (points.length > 0) segments.push(points);
    if (series.length === 1 && segments.length === 1 && segments[0].length > 1) {
      points = segments[0];
      const area = `${points.map(([x, y], index) => `${index === 0 ? "M" : "L"}${x} ${y}`).join(" ")} L${points.at(-1)[0]} ${zeroY} L${points[0][0]} ${zeroY} Z`;
      parts.push(`<defs><linearGradient id="lineArea" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${color}" stop-opacity=".22"/><stop offset="1" stop-color="${color}" stop-opacity="0"/></linearGradient></defs><path d="${area}" fill="url(#lineArea)"/>`);
    }
    for (const segment of segments) {
      if (segment.length > 1) {
        const path = segment.map(([x, y], index) => `${index === 0 ? "M" : "L"}${x} ${y}`).join(" ");
        parts.push(`<path d="${path}" fill="none" stroke="${escapeXML(color)}" stroke-width="${LINE_LAYOUT.lineWidth}" stroke-linecap="round" stroke-linejoin="round"/>`);
      }
      for (const [x, y] of segment) parts.push(`<circle cx="${x}" cy="${y}" r="${LINE_LAYOUT.pointRadius}" fill="${COLORS.canvas}" stroke="${escapeXML(color)}" stroke-width="${STROKE.pointWidth}"/>`);
    }
  });
  parts.push(layout.svg);
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render a stacked bar-chart SVG.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function stackedBarChart(chart, fonts) {
  const series = chart.series ?? [];
  const labels = (chart.labels ?? []).map(String);
  const legend = renderStackedBarLegend(series, fonts.regular);
  const { left, right, bottom } = STACKED_BAR_LAYOUT;
  const top = Math.max(STACKED_BAR_LAYOUT.minimumTop, legend.bottom + STACKED_BAR_LAYOUT.legendGap);
  const totals = labels.map((_, index) => series.reduce((sum, item) => sum + finiteNumber(item.values?.[index]), 0));
  const maximum = Math.max(STACKED_BAR_LAYOUT.minimumCountMaximum, niceMaximum(Math.max(0, ...totals)));
  const parts = [legend.svg];
  for (let index = 0; index <= STACKED_BAR_LAYOUT.gridSteps; index += 1) {
    const y = top + index * (bottom - top) / STACKED_BAR_LAYOUT.gridSteps;
    const value = maximum * (STACKED_BAR_LAYOUT.gridSteps - index) / STACKED_BAR_LAYOUT.gridSteps;
    parts.push(`<line class="grid" x1="${left}" y1="${y}" x2="${right}" y2="${y}"/>`);
    parts.push(`<text class="axis" x="${left - STACKED_BAR_LAYOUT.tickGap}" y="${y + 5}" text-anchor="end">${escapeXML(compactNumber(value))}</text>`);
  }
  if (chart.unit) {
    parts.push(`<text class="axis" transform="translate(${STACKED_BAR_LAYOUT.axisTitleX} ${STACKED_BAR_LAYOUT.axisTitleY}) rotate(-90)" text-anchor="middle">${escapeXML(chart.unit)}</text>`);
  }
  parts.push(`<line class="axis-line" x1="${left}" y1="${top}" x2="${left}" y2="${bottom}"/><line class="axis-line" x1="${left}" y1="${bottom}" x2="${right}" y2="${bottom}"/>`);
  const gap = (right - left) / Math.max(labels.length, 1);
  const barWidth = Math.min(STACKED_BAR_LAYOUT.maximumBarWidth, Math.max(STACKED_BAR_LAYOUT.minimumBarWidth, gap * STACKED_BAR_LAYOUT.barWidthRatio));
  labels.forEach((label, labelIndex) => {
    const x = left + labelIndex * gap + (gap - barWidth) / 2;
    let y = bottom;
    series.forEach((item, seriesIndex) => {
      const height = finiteNumber(item.values?.[labelIndex]) / maximum * (bottom - top);
      y -= height;
      parts.push(`<rect x="${x}" y="${y}" width="${barWidth}" height="${Math.max(0, height)}" fill="${escapeXML(item.color || CHART_COLORS[seriesIndex % CHART_COLORS.length])}"/>`);
    });
    const center = x + barWidth / 2;
    const labelWidth = Math.max(gap - STACKED_BAR_LAYOUT.labelGap, STACKED_BAR_LAYOUT.minimumLabelWidth);
    const text = fitText(fonts.regular, label, FONT.axisSize, Math.max(labelWidth, STACKED_BAR_LAYOUT.rotatedLabelMaximumWidth));
    parts.push(`<text class="axis" transform="translate(${center} ${STACKED_BAR_LAYOUT.axisLabelY}) rotate(${STACKED_BAR_LAYOUT.rotatedLabelAngle})" text-anchor="end">${escapeXML(text)}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold, {
    x: STACKED_BAR_LAYOUT.titleCenterX,
    y: STACKED_BAR_LAYOUT.titleCenterY,
    anchor: "middle",
    maximumWidth: STACKED_BAR_LAYOUT.titleMaximumWidth,
    panelHeader: true,
  });
}

/**
 * Render a grouped-column chart with the value-free Figma column treatment.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function groupedColumnChart(chart, fonts) {
  const series = chart.series ?? [];
  const labels = (chart.labels ?? []).map(String);
  const legend = renderLegend(series, fonts.regular);
  const top = Math.max(GROUPED_COLUMN_LAYOUT.minimumTop, legend.bottom + GROUPED_COLUMN_LAYOUT.legendGap);
  const visibleValues = series.flatMap((item) => (item.values ?? []).flatMap((value, index) => pointAvailable(item, index) ? [Math.max(0, finiteNumber(value))] : []));
  const maximum = niceMaximum(Math.max(0, ...visibleValues));
  const ticks = Array.from({ length: GROUPED_COLUMN_LAYOUT.gridSteps + 1 }, (_, index) => axisTick(maximum * (GROUPED_COLUMN_LAYOUT.gridSteps - index) / GROUPED_COLUMN_LAYOUT.gridSteps, chart.unit));
  const layout = cartesianLayout(labels, fonts.regular, GROUPED_COLUMN_LAYOUT, ticks, [], true);
  const { left, right, bottom } = layout;
  const parts = [legend.svg];
  for (let index = 0; index <= GROUPED_COLUMN_LAYOUT.gridSteps; index += 1) {
    const y = top + index * (bottom - top) / GROUPED_COLUMN_LAYOUT.gridSteps;
    parts.push(`<line class="grid" x1="${left}" y1="${y}" x2="${right}" y2="${y}"/>`);
    parts.push(`<text class="axis" data-role="left-axis-tick" x="${left - LABEL_LAYOUT.tickGap}" y="${y + 5}" text-anchor="end">${escapeXML(ticks[index])}</text>`);
  }
  parts.push(`<line class="axis-line" x1="${left}" y1="${top}" x2="${left}" y2="${bottom}"/><line class="axis-line" x1="${left}" y1="${bottom}" x2="${right}" y2="${bottom}"/>`);
  const categoryWidth = (right - left) / Math.max(labels.length, 1);
  const groupWidth = categoryWidth * GROUPED_COLUMN_LAYOUT.groupWidthRatio;
  const availableWidth = Math.max(1, groupWidth - GROUPED_COLUMN_LAYOUT.barGap * Math.max(series.length - 1, 0));
  const barWidth = Math.min(GROUPED_COLUMN_LAYOUT.maximumBarWidth, Math.max(GROUPED_COLUMN_LAYOUT.minimumBarWidth, availableWidth / Math.max(series.length, 1)));
  labels.forEach((label, labelIndex) => {
    const actualGroupWidth = barWidth * series.length + GROUPED_COLUMN_LAYOUT.barGap * Math.max(series.length - 1, 0);
    const groupLeft = left + labelIndex * categoryWidth + (categoryWidth - actualGroupWidth) / 2;
    series.forEach((item, seriesIndex) => {
      if (!pointAvailable(item, labelIndex)) return;
      const value = Math.max(0, finiteNumber(item.values?.[labelIndex]));
      const height = value / maximum * (bottom - top);
      const x = groupLeft + seriesIndex * (barWidth + GROUPED_COLUMN_LAYOUT.barGap);
      const y = bottom - height;
      const color = item.color || CHART_COLORS[seriesIndex % CHART_COLORS.length];
      parts.push(`<rect x="${x}" y="${y}" width="${barWidth}" height="${Math.max(0, height)}" rx="2" fill="${escapeXML(color)}"/>`);
    });
  });
  parts.push(layout.svg);
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render a horizontal bar-chart SVG.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function horizontalBarChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const values = chart.series?.[0]?.values ?? [];
  const color = chart.series?.[0]?.color || CHART_COLORS[0];
  const { left, right, top, bottom } = HORIZONTAL_BAR_LAYOUT;
  const rowHeight = (bottom - top) / Math.max(labels.length, 1);
  const barHeight = Math.min(HORIZONTAL_BAR_LAYOUT.maximumBarHeight, rowHeight * HORIZONTAL_BAR_LAYOUT.barHeightRatio);
  const maximum = niceMaximum(Math.max(0, ...values.map(finiteNumber)));
  const parts = [];
  for (let index = 0; index <= HORIZONTAL_BAR_LAYOUT.gridSteps; index += 1) {
    const x = left + index * (right - left) / HORIZONTAL_BAR_LAYOUT.gridSteps;
    parts.push(`<line class="grid" x1="${x}" y1="${top}" x2="${x}" y2="${bottom}"/>`);
  }
  parts.push(`<line class="axis-line" x1="${left}" y1="${top}" x2="${left}" y2="${bottom}"/><line class="axis-line" x1="${left}" y1="${bottom}" x2="${right}" y2="${bottom}"/>`);
  labels.forEach((label, index) => {
    const centerY = top + (index + 0.5) * rowHeight;
    const width = finiteNumber(values[index]) / maximum * (right - left);
    const fitted = fitWrappedText(
      fonts.regular,
      label,
      FONT.axisSize,
      HORIZONTAL_BAR_LAYOUT.labelMinimumFontSize,
      left - HORIZONTAL_BAR_LAYOUT.labelWidthGap,
      HORIZONTAL_BAR_LAYOUT.labelMaximumLines,
    );
    parts.push(multilineText(
      fitted.lines,
      left - HORIZONTAL_BAR_LAYOUT.labelRightGap,
      centerY,
      fitted.size,
      fitted.size * HORIZONTAL_BAR_LAYOUT.labelLineHeightRatio,
      `class="axis" text-anchor="end" style="font-size:${fitted.size}px" data-role="horizontal-label"`,
    ));
    parts.push(`<rect x="${left}" y="${centerY - barHeight / 2}" width="${Math.max(0, width)}" height="${barHeight}" fill="${escapeXML(color)}"/>`);
    parts.push(`<text class="axis" x="${Math.min(right - HORIZONTAL_BAR_LAYOUT.valueRightPadding, left + width + HORIZONTAL_BAR_LAYOUT.valueGap)}" y="${centerY + 5}" text-anchor="${left + width + HORIZONTAL_BAR_LAYOUT.valueFlipThreshold > right ? "end" : "start"}">${escapeXML(compactNumber(values[index]))}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Convert polar coordinates to Cartesian coordinates.
 * @param {number} centerX Circle center x-coordinate.
 * @param {number} centerY Circle center y-coordinate.
 * @param {number} radius Radius in pixels.
 * @param {number} angle Angle in radians.
 * @returns {[number, number]} Cartesian point.
 */
function polarPoint(centerX, centerY, radius, angle) {
  return [centerX + radius * Math.cos(angle), centerY + radius * Math.sin(angle)];
}

/**
 * Build the SVG path for one donut segment.
 * @param {number} centerX Circle center x-coordinate.
 * @param {number} centerY Circle center y-coordinate.
 * @param {number} outer Outer radius.
 * @param {number} inner Inner radius.
 * @param {number} start Start angle in radians.
 * @param {number} end End angle in radians.
 * @returns {string} SVG path data.
 */
function donutPath(centerX, centerY, outer, inner, start, end) {
  const [outerStartX, outerStartY] = polarPoint(centerX, centerY, outer, start);
  const [outerEndX, outerEndY] = polarPoint(centerX, centerY, outer, end);
  const [innerEndX, innerEndY] = polarPoint(centerX, centerY, inner, end);
  const [innerStartX, innerStartY] = polarPoint(centerX, centerY, inner, start);
  const largeArc = end - start > Math.PI ? 1 : 0;
  return `M${outerStartX} ${outerStartY} A${outer} ${outer} 0 ${largeArc} 1 ${outerEndX} ${outerEndY} L${innerEndX} ${innerEndY} A${inner} ${inner} 0 ${largeArc} 0 ${innerStartX} ${innerStartY} Z`;
}

/**
 * Render a donut-chart SVG.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function donutChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const values = chart.series?.[0]?.values?.map(finiteNumber) ?? [];
  const total = values.reduce((sum, value) => sum + Math.max(0, value), 0);
  const centerX = DONUT_LAYOUT.centerX;
  const centerY = DONUT_LAYOUT.centerY;
  const outer = DONUT_LAYOUT.outerRadius;
  const inner = DONUT_LAYOUT.innerRadius;
  const parts = [];
  const positiveIndexes = values.map((value, index) => value > 0 ? index : -1).filter((index) => index >= 0);
  if (total > 0 && positiveIndexes.length === 1) {
    const color = categoricalColor(chart, positiveIndexes[0]);
    parts.push(`<circle cx="${centerX}" cy="${centerY}" r="${(outer + inner) / 2}" fill="none" stroke="${escapeXML(color)}" stroke-width="${outer - inner}"/>`);
  } else {
    let angle = -Math.PI / 2;
    values.forEach((value, index) => {
      const next = total > 0 ? angle + Math.max(0, value) / total * Math.PI * 2 : angle;
      if (next > angle) parts.push(`<path d="${donutPath(centerX, centerY, outer, inner, angle, next)}" fill="${escapeXML(categoricalColor(chart, index))}"/>`);
      angle = next;
    });
  }
  const centerValue = String(chart.centerValue || compactNumber(total));
  const centerLabel = String(chart.centerLabel || TEXT.riskTotal);
  parts.push(`<text x="${centerX}" y="${DONUT_LAYOUT.totalValueY}" text-anchor="middle" style="font-size:${FONT.donutValueSize}px;font-weight:700;fill:${COLORS.inkText}">${escapeXML(centerValue)}</text>`);
  parts.push(`<text x="${centerX}" y="${DONUT_LAYOUT.totalLabelY}" text-anchor="middle" style="font-size:${FONT.percentageSize}px;fill:${COLORS.mutedText}">${escapeXML(centerLabel)}</text>`);
  const legendX = DONUT_LAYOUT.legendX;
  const rowHeight = Math.min(DONUT_LAYOUT.legendMaximumRowHeight, DONUT_LAYOUT.legendMaximumHeight / Math.max(labels.length, 1));
  labels.forEach((label, index) => {
    const y = DONUT_LAYOUT.legendTop + index * rowHeight;
    const percentage = total > 0 ? values[index] / total * 100 : 0;
    const text = fitText(fonts.regular, label, FONT.legendSize, DONUT_LAYOUT.legendTextMaximumWidth);
    parts.push(`<circle cx="${legendX}" cy="${y - DONUT_LAYOUT.legendMarkerOffsetY}" r="${DONUT_LAYOUT.legendMarkerRadius}" fill="${escapeXML(categoricalColor(chart, index))}"/>`);
    parts.push(`<text class="legend" x="${legendX + DONUT_LAYOUT.legendTextGap}" y="${y}">${escapeXML(text)}</text>`);
    parts.push(`<text x="${DONUT_LAYOUT.percentageX}" y="${y}" text-anchor="end" style="font-size:${FONT.percentageSize}px;font-weight:700;fill:${COLORS.inkText}">${escapeXML(`${percentage.toFixed(0)}%`)}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold, { panelHeader: true, hideTitle: true });
}

/**
 * Build the SVG path for one pie segment.
 * @param {number} centerX Circle center x-coordinate.
 * @param {number} centerY Circle center y-coordinate.
 * @param {number} radius Segment radius.
 * @param {number} start Start angle in radians.
 * @param {number} end End angle in radians.
 * @returns {string} SVG path data.
 */
function piePath(centerX, centerY, radius, start, end) {
  const [startX, startY] = polarPoint(centerX, centerY, radius, start);
  const [endX, endY] = polarPoint(centerX, centerY, radius, end);
  const largeArc = end - start > Math.PI ? 1 : 0;
  return `M${centerX} ${centerY} L${startX} ${startY} A${radius} ${radius} 0 ${largeArc} 1 ${endX} ${endY} Z`;
}

/**
 * Render a pie chart using the component-library composition and legend.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function pieChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const values = chart.series?.[0]?.values?.map((value) => Math.max(0, finiteNumber(value))) ?? [];
  const total = values.reduce((sum, value) => sum + value, 0);
  const parts = [];
  const positiveIndexes = values.map((value, index) => value > 0 ? index : -1).filter((index) => index >= 0);
  if (total > 0 && positiveIndexes.length === 1) {
    const color = categoricalColor(chart, positiveIndexes[0]);
    parts.push(`<circle cx="${PIE_LAYOUT.centerX}" cy="${PIE_LAYOUT.centerY}" r="${PIE_LAYOUT.radius}" fill="${escapeXML(color)}"/>`);
  } else {
    let angle = -Math.PI / 2;
    values.forEach((value, index) => {
      const next = total > 0 ? angle + value / total * Math.PI * 2 : angle;
      if (next > angle) parts.push(`<path d="${piePath(PIE_LAYOUT.centerX, PIE_LAYOUT.centerY, PIE_LAYOUT.radius, angle, next)}" fill="${escapeXML(categoricalColor(chart, index))}" stroke="${COLORS.canvas}" stroke-width="${STROKE.pieWidth}"/>`);
      angle = next;
    });
  }
  const legendX = DONUT_LAYOUT.legendX;
  const rowHeight = Math.min(DONUT_LAYOUT.legendMaximumRowHeight, DONUT_LAYOUT.legendMaximumHeight / Math.max(labels.length, 1));
  labels.forEach((label, index) => {
    const y = DONUT_LAYOUT.legendTop + index * rowHeight;
    const percentage = total > 0 ? values[index] / total * 100 : 0;
    parts.push(`<circle cx="${legendX}" cy="${y - DONUT_LAYOUT.legendMarkerOffsetY}" r="${DONUT_LAYOUT.legendMarkerRadius}" fill="${escapeXML(categoricalColor(chart, index))}"/>`);
    parts.push(`<text class="legend" x="${legendX + DONUT_LAYOUT.legendTextGap}" y="${y}">${escapeXML(fitText(fonts.regular, label, FONT.legendSize, DONUT_LAYOUT.legendTextMaximumWidth))}</text>`);
    parts.push(`<text x="${DONUT_LAYOUT.percentageX}" y="${y}" text-anchor="end" style="font-size:${FONT.percentageSize}px;font-weight:700;fill:${COLORS.inkText}">${escapeXML(`${percentage.toFixed(1)}%`)}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render bars on the left axis and lines on the right axis in one chart.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function comboChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const series = chart.series ?? [];
  const barSeries = series.filter((item) => item.renderAs === "bar");
  const lineSeries = series.filter((item) => !barSeries.includes(item));
  const leftSeries = series.filter((item) => item.axis !== "right");
  const rightSeries = series.filter((item) => item.axis === "right");
  const legend = renderLegend(series, fonts.regular);
  const top = Math.max(GROUPED_COLUMN_LAYOUT.minimumTop, legend.bottom + GROUPED_COLUMN_LAYOUT.legendGap);
  const valuesFor = (items) => items.flatMap((item) => (item.values ?? []).flatMap((value, index) => pointAvailable(item, index) ? [finiteNumber(value)] : []));
  const leftBounds = niceAxisBounds(valuesFor(leftSeries));
  const rightBounds = niceAxisBounds(valuesFor(rightSeries));
  const ticksFor = (bounds, unit) => Array.from({ length: GROUPED_COLUMN_LAYOUT.gridSteps + 1 }, (_, index) => axisTick(bounds.maximum - index * (bounds.maximum - bounds.minimum) / GROUPED_COLUMN_LAYOUT.gridSteps, unit));
  const leftTicks = ticksFor(leftBounds, chart.unit);
  const rightTicks = rightSeries.length ? ticksFor(rightBounds, chart.secondaryUnit) : [];
  const layout = cartesianLayout(labels, fonts.regular, GROUPED_COLUMN_LAYOUT, leftTicks, rightTicks, true);
  const { left, right, bottom } = layout;
  const parts = [legend.svg];
  for (let index = 0; index <= GROUPED_COLUMN_LAYOUT.gridSteps; index += 1) {
    const y = top + index * (bottom - top) / GROUPED_COLUMN_LAYOUT.gridSteps;
    parts.push(`<line class="grid" x1="${left}" y1="${y}" x2="${right}" y2="${y}"/>`);
    parts.push(`<text class="axis" data-role="left-axis-tick" x="${left - LABEL_LAYOUT.tickGap}" y="${y + 5}" text-anchor="end">${escapeXML(leftTicks[index])}</text>`);
    if (rightSeries.length > 0) {
      parts.push(`<text class="axis" data-role="right-axis-tick" x="${right + LABEL_LAYOUT.tickGap}" y="${y + 5}" text-anchor="start">${escapeXML(rightTicks[index])}</text>`);
    }
  }
  const leftZeroY = axisY(0, leftBounds, top, bottom);
  parts.push(`<line class="axis-line" x1="${left}" y1="${top}" x2="${left}" y2="${bottom}"/>`);
  if (rightSeries.length > 0) parts.push(`<line class="axis-line" x1="${right}" y1="${top}" x2="${right}" y2="${bottom}"/>`);
  parts.push(`<line class="axis-line" x1="${left}" y1="${leftZeroY}" x2="${right}" y2="${leftZeroY}"/>`);
  const categoryWidth = (right - left) / Math.max(labels.length, 1);
  const barWidth = Math.min(GROUPED_COLUMN_LAYOUT.maximumBarWidth, Math.max(GROUPED_COLUMN_LAYOUT.minimumBarWidth, categoryWidth * 0.42 / Math.max(barSeries.length, 1)));
  labels.forEach((label, labelIndex) => {
    const groupWidth = barWidth * barSeries.length + GROUPED_COLUMN_LAYOUT.barGap * Math.max(barSeries.length - 1, 0);
    const groupLeft = left + labelIndex * categoryWidth + (categoryWidth - groupWidth) / 2;
    barSeries.forEach((item, seriesIndex) => {
      if (!pointAvailable(item, labelIndex)) return;
      const valueY = axisY(item.values?.[labelIndex], leftBounds, top, bottom);
      const y = Math.min(valueY, leftZeroY);
      const height = Math.abs(valueY - leftZeroY);
      parts.push(`<rect x="${groupLeft + seriesIndex * (barWidth + GROUPED_COLUMN_LAYOUT.barGap)}" y="${y}" width="${barWidth}" height="${height}" rx="2" fill="${escapeXML(item.color || CHART_COLORS[seriesIndex % CHART_COLORS.length])}"/>`);
    });
  });
  lineSeries.forEach((item, seriesIndex) => {
    const color = item.color || CHART_COLORS[(seriesIndex + barSeries.length) % CHART_COLORS.length];
    const bounds = item.axis === "right" ? rightBounds : leftBounds;
    const axisName = item.axis === "right" ? "right" : "left";
    const segments = [];
    let points = [];
    labels.forEach((_, labelIndex) => {
      if (!pointAvailable(item, labelIndex)) {
        if (points.length > 0) segments.push(points);
        points = [];
        return;
      }
      const x = left + (labelIndex + 0.5) * categoryWidth;
      points.push([x, axisY(item.values?.[labelIndex], bounds, top, bottom)]);
    });
    if (points.length > 0) segments.push(points);
    segments.forEach((segment) => {
      if (segment.length > 1) parts.push(`<path d="${segment.map(([x, y], index) => `${index === 0 ? "M" : "L"}${x} ${y}`).join(" ")}" fill="none" stroke="${escapeXML(color)}" stroke-width="${STROKE.combinedLineWidth}" stroke-linecap="round" stroke-linejoin="round"/>`);
      segment.forEach(([x, y]) => parts.push(`<circle data-role="combo-line-point" data-axis="${axisName}" cx="${x}" cy="${y}" r="${LINE_LAYOUT.pointRadius}" fill="${COLORS.canvas}" stroke="${escapeXML(color)}" stroke-width="${STROKE.pointWidth}"/>`));
    });
  });
  parts.push(layout.svg);
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render a radar chart with one or more normalized comparison polygons.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function radarChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const series = chart.series ?? [];
  const maximum = niceMaximum(Math.max(0, ...series.flatMap((item) => item.values ?? [])));
  const parts = [];
  for (let step = 1; step <= RADAR_LAYOUT.gridSteps; step += 1) {
    const radius = RADAR_LAYOUT.radius * step / RADAR_LAYOUT.gridSteps;
    const polygon = labels.map((_, index) => polarPoint(RADAR_LAYOUT.centerX, RADAR_LAYOUT.centerY, radius, -Math.PI / 2 + index * Math.PI * 2 / labels.length).join(",")).join(" ");
    parts.push(`<polygon points="${polygon}" fill="none" stroke="${COLORS.grid}" stroke-width="${STROKE.radarGridWidth}"/>`);
  }
  labels.forEach((label, index) => {
    const angle = -Math.PI / 2 + index * Math.PI * 2 / labels.length;
    const [axisX, axisY] = polarPoint(RADAR_LAYOUT.centerX, RADAR_LAYOUT.centerY, RADAR_LAYOUT.radius, angle);
    const [labelX, labelY] = polarPoint(RADAR_LAYOUT.centerX, RADAR_LAYOUT.centerY, RADAR_LAYOUT.labelRadius, angle);
    parts.push(`<line x1="${RADAR_LAYOUT.centerX}" y1="${RADAR_LAYOUT.centerY}" x2="${axisX}" y2="${axisY}" stroke="${COLORS.grid}"/>`);
    parts.push(`<text class="axis" x="${labelX}" y="${labelY + 5}" text-anchor="${Math.abs(labelX - RADAR_LAYOUT.centerX) < 10 ? "middle" : labelX < RADAR_LAYOUT.centerX ? "end" : "start"}">${escapeXML(fitText(fonts.regular, label, FONT.axisSize, RADAR_LAYOUT.labelMaximumWidth))}</text>`);
  });
  series.forEach((item, seriesIndex) => {
    const color = item.color || CHART_COLORS[seriesIndex % CHART_COLORS.length];
    const polygon = labels.map((_, index) => {
      const radius = Math.max(0, finiteNumber(item.values?.[index])) / maximum * RADAR_LAYOUT.radius;
      return polarPoint(RADAR_LAYOUT.centerX, RADAR_LAYOUT.centerY, radius, -Math.PI / 2 + index * Math.PI * 2 / labels.length).join(",");
    }).join(" ");
    parts.push(`<polygon points="${polygon}" fill="${escapeXML(color)}" fill-opacity=".14" stroke="${escapeXML(color)}" stroke-width="${STROKE.radarSeriesWidth}"/>`);
    parts.push(`<rect x="${RADAR_LAYOUT.legendLeft}" y="${RADAR_LAYOUT.legendTop + seriesIndex * RADAR_LAYOUT.legendRowHeight - 12}" width="14" height="14" rx="2" fill="${escapeXML(color)}"/><text class="legend" x="${RADAR_LAYOUT.legendLeft + 24}" y="${RADAR_LAYOUT.legendTop + seriesIndex * RADAR_LAYOUT.legendRowHeight}">${escapeXML(fitText(fonts.regular, item.name, FONT.legendSize, 330))}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render scatter points on a four-quadrant or remediation-priority matrix.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function quadrantChart(chart, fonts) {
  const points = (chart.points ?? []).filter((point) => point.available !== false);
  const { left, right, top, bottom } = QUADRANT_LAYOUT;
  const maximumX = Math.max(QUADRANT_LAYOUT.minimumScaleMaximum, ...points.map((point) => finiteNumber(point.x)));
  const maximumY = Math.max(QUADRANT_LAYOUT.minimumScaleMaximum, ...points.map((point) => finiteNumber(point.y)));
  const middleX = left + (right - left) * QUADRANT_LAYOUT.midpointX;
  const middleY = bottom - (bottom - top) * QUADRANT_LAYOUT.midpointY;
  const parts = [
    `<rect x="${left}" y="${top}" width="${right - left}" height="${bottom - top}" fill="${COLORS.quadrantBackground}" stroke="${COLORS.frameBorder}"/>`,
    `<rect x="${middleX}" y="${top}" width="${right - middleX}" height="${middleY - top}" fill="${COLORS.quadrantPriority}"/>`,
    `<rect x="${left}" y="${top}" width="${middleX - left}" height="${middleY - top}" fill="${COLORS.quadrantObserve}"/>`,
    `<rect x="${middleX}" y="${middleY}" width="${right - middleX}" height="${bottom - middleY}" fill="${COLORS.quadrantQuick}"/>`,
    `<line x1="${middleX}" y1="${top}" x2="${middleX}" y2="${bottom}" stroke="${COLORS.quadrantAxis}" stroke-dasharray="${STROKE.quadrantDash}"/>`,
    `<line x1="${left}" y1="${middleY}" x2="${right}" y2="${middleY}" stroke="${COLORS.quadrantAxis}" stroke-dasharray="${STROKE.quadrantDash}"/>`,
    `<text class="axis" x="${left + QUADRANT_LAYOUT.quadrantPadding}" y="${top + 24}">${escapeXML(TEXT.quadrantHighImpact)}</text>`,
    `<text class="axis" x="${right - QUADRANT_LAYOUT.quadrantPadding}" y="${top + 24}" text-anchor="end">${escapeXML(TEXT.quadrantPriority)}</text>`,
    `<text class="axis" x="${left + QUADRANT_LAYOUT.quadrantPadding}" y="${bottom - 12}">${escapeXML(TEXT.quadrantObserve)}</text>`,
    `<text class="axis" x="${right - QUADRANT_LAYOUT.quadrantPadding}" y="${bottom - 12}" text-anchor="end">${escapeXML(TEXT.quadrantQuick)}</text>`,
  ];
  const occupiedLabels = [];
  points.forEach((point, index) => {
    const x = left + Math.max(0, finiteNumber(point.x)) / maximumX * (right - left);
    const y = bottom - Math.max(0, finiteNumber(point.y)) / maximumY * (bottom - top);
    const color = point.color || (point.level === "高风险" ? COLORS.highRisk : point.level === "中风险" ? COLORS.mediumRisk : point.level === "低风险" ? COLORS.lowRisk : CHART_COLORS[index % CHART_COLORS.length]);
    const label = fitText(fonts.regular, point.label, FONT.axisSize, QUADRANT_LAYOUT.labelMaximumWidth);
    const placement = quadrantLabelPlacement(fonts.regular, label, x, y, occupiedLabels);
    parts.push(`<circle cx="${x}" cy="${y}" r="${QUADRANT_LAYOUT.pointRadius}" fill="${escapeXML(color)}" fill-opacity=".9" stroke="${COLORS.canvas}" stroke-width="${STROKE.pointWidth}"/>`);
    parts.push(`<text class="axis" x="${placement.x}" y="${placement.y}" text-anchor="${placement.placeLeft ? "end" : "start"}">${escapeXML(label)}</text>`);
  });
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Place one matrix label inside the plotting area without overlapping earlier
 * labels. Parameters are the packaged font, fitted label, point coordinates,
 * and mutable occupied rectangles. Returns SVG text coordinates and anchor.
 */
function quadrantLabelPlacement(font, label, pointX, pointY, occupiedLabels) {
  const labelWidth = measure(font, label, FONT.axisSize);
  const preferredLeft = pointX + QUADRANT_LAYOUT.labelOffsetX + labelWidth > QUADRANT_LAYOUT.right - QUADRANT_LAYOUT.labelBoundaryPadding;
  const sideCandidates = preferredLeft ? [true, false] : [false, true];
  const minimumBaseline = QUADRANT_LAYOUT.labelBaselineMinimum;
  const maximumBaseline = QUADRANT_LAYOUT.bottom - QUADRANT_LAYOUT.labelBaselineBottomPadding;

  for (const offsetY of QUADRANT_LAYOUT.labelVerticalCandidates) {
    const baseline = Math.min(maximumBaseline, Math.max(minimumBaseline, pointY + offsetY));
    for (const placeLeft of sideCandidates) {
      const x = placeLeft ? pointX - QUADRANT_LAYOUT.labelOffsetX : pointX + QUADRANT_LAYOUT.labelOffsetX;
      const rectangle = {
        left: placeLeft ? x - labelWidth : x,
        right: placeLeft ? x : x + labelWidth,
        top: baseline - FONT.axisSize,
        bottom: baseline + QUADRANT_LAYOUT.labelCollisionGap,
      };
      if (rectangle.left < QUADRANT_LAYOUT.left + QUADRANT_LAYOUT.labelBoundaryPadding ||
          rectangle.right > QUADRANT_LAYOUT.right - QUADRANT_LAYOUT.labelBoundaryPadding) {
        continue;
      }
      if (occupiedLabels.some((occupied) => rectanglesOverlap(rectangle, occupied, QUADRANT_LAYOUT.labelCollisionGap))) {
        continue;
      }
      occupiedLabels.push(rectangle);
      return { x, y: baseline, placeLeft };
    }
  }

  // Dense matrices can exhaust the preferred offsets. The final placement is
  // still clamped inside the plot and keeps the text visible.
  const placeLeft = preferredLeft;
  const x = placeLeft ? pointX - QUADRANT_LAYOUT.labelOffsetX : pointX + QUADRANT_LAYOUT.labelOffsetX;
  const y = Math.min(maximumBaseline, Math.max(minimumBaseline, pointY + QUADRANT_LAYOUT.labelOffsetY));
  occupiedLabels.push({
    left: placeLeft ? x - labelWidth : x,
    right: placeLeft ? x : x + labelWidth,
    top: y - FONT.axisSize,
    bottom: y + QUADRANT_LAYOUT.labelCollisionGap,
  });
  return { x, y, placeLeft };
}

/**
 * Test whether two label rectangles overlap after applying a safety gap.
 * Parameters are two rectangles and the required gap. Returns true on overlap.
 */
function rectanglesOverlap(first, second, gap) {
  return first.left < second.right + gap &&
    first.right + gap > second.left &&
    first.top < second.bottom + gap &&
    first.bottom + gap > second.top;
}

/**
 * Render a verified enterprise-relationship network SVG.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function networkChart(chart, fonts) {
  const labels = (chart.labels ?? []).map(String);
  const values = chart.series?.[0]?.values ?? [];
  const root = labels[0] ?? "企业";
  const children = labels.slice(1, NETWORK_LAYOUT.maximumChildren + 1);
  const parts = [];
  const rootY = NETWORK_LAYOUT.rootY;
  const gap = children.length > 1 ? NETWORK_LAYOUT.childRange / (children.length - 1) : 0;
  children.forEach((label, index) => {
    const y = children.length === 1 ? rootY : NETWORK_LAYOUT.childTop + index * gap;
    parts.push(`<path d="M${NETWORK_LAYOUT.pathRootX} ${rootY} C${NETWORK_LAYOUT.pathControlX} ${rootY} ${NETWORK_LAYOUT.pathControlX} ${y} ${NETWORK_LAYOUT.pathChildX} ${y}" fill="none" stroke="${COLORS.networkLine}" stroke-width="${STROKE.networkPathWidth}"/>`);
    const percentage = pointAvailable(chart.series?.[0] ?? {}, index + 1) ? finiteNumber(values[index + 1]) : 0;
    const edgeLabel = percentage > 0 ? `${percentage.toFixed(2)}%` : "比例未获取";
    parts.push(`<rect x="${NETWORK_LAYOUT.edgeLabelLeft}" y="${(rootY + y) / 2 - NETWORK_LAYOUT.edgeLabelHeight / 2}" width="${NETWORK_LAYOUT.edgeLabelWidth}" height="${NETWORK_LAYOUT.edgeLabelHeight}" rx="${NETWORK_LAYOUT.edgeLabelRadius}" fill="${COLORS.networkLabelBackground}"/><text class="axis" x="${NETWORK_LAYOUT.edgeLabelCenterX}" y="${(rootY + y) / 2 + 5}" text-anchor="middle">${escapeXML(edgeLabel)}</text>`);
    const fittedChild = fitWrappedText(fonts.regular, label, FONT.networkChildSize, NETWORK_LAYOUT.nodeMinimumFontSize, NETWORK_LAYOUT.nodeTextMaximumWidth, NETWORK_LAYOUT.nodeMaximumLines);
    parts.push(`<rect x="${NETWORK_LAYOUT.childLeft}" y="${y - NETWORK_LAYOUT.childHeight / 2}" width="${NETWORK_LAYOUT.childWidth}" height="${NETWORK_LAYOUT.childHeight}" rx="${NETWORK_LAYOUT.childRadius}" fill="${COLORS.canvas}" stroke="${COLORS.networkNodeBorder}" stroke-width="${STROKE.networkNodeWidth}"/>${multilineText(fittedChild.lines, NETWORK_LAYOUT.childCenterX, y, fittedChild.size, fittedChild.size * NETWORK_LAYOUT.nodeLineHeightRatio, `text-anchor="middle" style="font-size:${fittedChild.size}px;fill:${COLORS.inkText}" data-role="network-child-label"`)}`);
  });
  const fittedRoot = fitWrappedText(fonts.bold, root, FONT.networkRootSize, NETWORK_LAYOUT.nodeMinimumFontSize, NETWORK_LAYOUT.nodeTextMaximumWidth, NETWORK_LAYOUT.nodeMaximumLines);
  parts.push(`<rect x="${NETWORK_LAYOUT.rootLeft}" y="${rootY - NETWORK_LAYOUT.rootHeight / 2}" width="${NETWORK_LAYOUT.rootWidth}" height="${NETWORK_LAYOUT.rootHeight}" rx="${NETWORK_LAYOUT.rootRadius}" fill="${COLORS.paleBlue}" stroke="${COLORS.primary}" stroke-width="${STROKE.networkRootWidth}"/>${multilineText(fittedRoot.lines, NETWORK_LAYOUT.rootX, rootY, fittedRoot.size, fittedRoot.size * NETWORK_LAYOUT.nodeLineHeightRatio, `text-anchor="middle" style="font-size:${fittedRoot.size}px;font-weight:700;fill:${COLORS.networkRootText}" data-role="network-root-label"`)}`);
  if (children.length === 0) parts.push(`<text x="${NETWORK_LAYOUT.emptyTextX}" y="${NETWORK_LAYOUT.emptyTextY}" text-anchor="middle" style="font-size:${FONT.networkEmptySize}px;fill:${COLORS.mutedText}">${escapeXML(TEXT.networkEmpty)}</text>`);
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Render the component-library enterprise-change timeline.
 * @param {object} chart Normalized chart model with dates in labels and event
 * names in matching series entries.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 */
function timelineChart(chart, fonts) {
  const dates = (chart.labels ?? []).map(String).slice(0, TIMELINE_LAYOUT.maximumItems);
  const events = (chart.series ?? []).slice(0, dates.length);
  const usableWidth = TIMELINE_LAYOUT.right - TIMELINE_LAYOUT.left;
  const gap = dates.length > 1 ? usableWidth / (dates.length - 1) : 0;
  const parts = [`<line x1="${TIMELINE_LAYOUT.left}" y1="${TIMELINE_LAYOUT.axisY}" x2="${TIMELINE_LAYOUT.right}" y2="${TIMELINE_LAYOUT.axisY}" stroke="${COLORS.networkLine}" stroke-width="3"/>`];
  dates.forEach((date, index) => {
    const x = dates.length === 1 ? (TIMELINE_LAYOUT.left + TIMELINE_LAYOUT.right) / 2 : TIMELINE_LAYOUT.left + index * gap;
    const above = index % 2 === 0;
    const stemEnd = above ? TIMELINE_LAYOUT.axisY - TIMELINE_LAYOUT.stemLength : TIMELINE_LAYOUT.axisY + TIMELINE_LAYOUT.stemLength;
    const event = events[index]?.name ?? "变更事项";
    const fitted = fitWrappedText(fonts.regular, event, FONT.axisSize, TIMELINE_LAYOUT.labelMinimumFontSize, TIMELINE_LAYOUT.labelWidth, TIMELINE_LAYOUT.labelMaximumLines);
    const labelY = above ? TIMELINE_LAYOUT.itemTopY : TIMELINE_LAYOUT.itemBottomY;
    const dateY = above ? TIMELINE_LAYOUT.axisY + TIMELINE_LAYOUT.dateOffsetY : TIMELINE_LAYOUT.axisY - TIMELINE_LAYOUT.dateOffsetY;
    parts.push(`<line x1="${x}" y1="${TIMELINE_LAYOUT.axisY}" x2="${x}" y2="${stemEnd}" stroke="${COLORS.primary}" stroke-width="2"/>`);
    parts.push(`<circle cx="${x}" cy="${TIMELINE_LAYOUT.axisY}" r="${TIMELINE_LAYOUT.pointRadius}" fill="${COLORS.canvas}" stroke="${COLORS.primary}" stroke-width="3"/>`);
    parts.push(`<text x="${x}" y="${dateY}" text-anchor="middle" style="font-size:${FONT.axisSize}px;fill:${COLORS.mutedText}">${escapeXML(date)}</text>`);
    parts.push(multilineText(fitted.lines, x, labelY, fitted.size, fitted.size * TIMELINE_LAYOUT.labelLineHeightRatio, `text-anchor="middle" style="font-size:${fitted.size}px;fill:${COLORS.inkText}" data-role="timeline-event-label"`));
  });
  return svgShell(chart.title, parts.join(""), fonts.bold);
}

/**
 * Dispatch a normalized chart model to its renderer.
 * @param {object} chart Normalized chart model.
 * @param {{regular: opentype.Font, bold: opentype.Font}} fonts Packaged fonts.
 * @returns {string} Complete SVG document.
 * @throws {Error} When the chart kind is unsupported.
 */
function renderSVG(chart, fonts) {
  if (!chartHasData(chart)) return emptyChart(chart, fonts);
  switch (chart.kind) {
    case "line": return lineChart(chart, fonts);
    case "multi_line": return lineChart(chart, fonts);
    case "grouped_column": return groupedColumnChart(chart, fonts);
    case "stacked_bar": return stackedBarChart(chart, fonts);
    case "horizontal_bar": return horizontalBarChart(chart, fonts);
    case "donut": return donutChart(chart, fonts);
    case "pie": return pieChart(chart, fonts);
    case "combo": return comboChart(chart, fonts);
    case "dual_axis": return comboChart(chart, fonts);
    case "radar": return radarChart(chart, fonts);
    case "quadrant": return quadrantChart(chart, fonts);
    case "priority_matrix": return quadrantChart(chart, fonts);
    case "network": return networkChart(chart, fonts);
    case "timeline": return timelineChart(chart, fonts);
    default: throw new Error(`unsupported chart kind ${JSON.stringify(chart.kind)}`);
  }
}

/**
 * Load packaged fonts, render stdin chart JSON and write SVG or PNG output.
 * @returns {Promise<void>} Resolves after the output file has been written.
 * @throws {Error} When arguments, fonts, input data or rendering are invalid.
 */
async function main() {
  // The packaged runtime invokes this module by absolute path. Scan the full
  // argument list; parseArgs ignores everything except explicit renderer flags.
  const args = parseArgs(process.argv);
  if (process.env.TAX_ANALYSIS_RENDERER_DEBUG === "1") {
    console.error(JSON.stringify({ argv: process.argv, args }));
  }
  if (!args["regular-font"] || !args["bold-font"]) throw new Error("--regular-font and --bold-font are required");
  const regularBytes = readFileSync(args["regular-font"]);
  const boldBytes = readFileSync(args["bold-font"]);
  const input = readFileSync(0, "utf8");
  if (regularBytes.length < 1024 || boldBytes.length < 1024) throw new Error("font files are empty or invalid");
  const chart = JSON.parse(input);
  const fonts = {
    regular: opentype.parse(regularBytes.buffer.slice(regularBytes.byteOffset, regularBytes.byteOffset + regularBytes.byteLength)),
    bold: opentype.parse(boldBytes.buffer.slice(boldBytes.byteOffset, boldBytes.byteOffset + boldBytes.byteLength)),
  };
  const svg = renderSVG(chart, fonts);
  if (args["svg-output-file"]) {
    writeFileSync(args["svg-output-file"], svg, "utf8");
    return;
  }
  const wasmBytes = readFileSync(wasmPath);
  await initWasm(wasmBytes);
  const renderer = new Resvg(svg, {
    fitTo: { mode: "width", value: CANVAS.width },
    background: COLORS.canvas,
    font: { fontBuffers: [regularBytes, boldBytes], loadSystemFonts: false, defaultFontFamily: FONT.family },
  });
  const png = renderer.render().asPng();
  if (args["output-file"]) writeFileSync(args["output-file"], png);
  else writeFileSync(1, png);
}

try {
  await main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
