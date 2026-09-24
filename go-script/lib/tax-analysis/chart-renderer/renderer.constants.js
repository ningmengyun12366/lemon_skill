export const CANVAS = Object.freeze({
  width: 1200,
  height: 500,
  frameInset: 1,
  frameWidth: 1198,
  frameHeight: 498,
});

export const CHART_COLORS = Object.freeze([
  "#3D7FFF",
  "#28B950",
  "#F86602",
  "#F7B500",
  "#52C7C2",
  "#8B5BDC",
  "#6574FF",
  "#E85D9E",
]);

export const COLORS = Object.freeze({
  canvas: "#FFFFFF",
  frameBorder: "#DCE2EB",
  panelHeader: "#F7F9FE",
  panelShadow: "#EEF2F7",
  bodyText: "#333333",
  headingText: "#535C69",
  mutedText: "#8A94A3",
  inkText: "#242A33",
  grid: "#D9DFE8",
  axisLine: "#CCCCCC",
  primary: "#3D7FFF",
  paleBlue: "#EAF2FF",
  emptyBackground: "#F8FAFD",
  emptyBorder: "#DCE5F2",
  highRisk: "#FF3B42",
  mediumRisk: "#F86602",
  lowRisk: "#F7B500",
  quadrantBackground: "#FAFCFF",
  quadrantPriority: "#FFF1F0",
  quadrantObserve: "#FFF8E8",
  quadrantQuick: "#EEF5FF",
  quadrantAxis: "#AAB4C3",
  networkLine: "#AFC7F4",
  networkLabelBackground: "#F2F6FD",
  networkNodeBorder: "#8FB2F3",
  networkRootText: "#1F5FD1",
});

export const STROKE = Object.freeze({
  gridWidth: 1,
  gridDash: "2 3",
  axisWidth: 1.2,
  emptyDash: "8 8",
  emptyIconWidth: 4,
  pointWidth: 2,
  combinedLineWidth: 3,
  pieWidth: 2,
  radarGridWidth: 1,
  radarSeriesWidth: 2.5,
  quadrantDash: "6 6",
  networkPathWidth: 2,
  networkNodeWidth: 1.5,
  networkRootWidth: 2,
});

export const FONT = Object.freeze({
  family: "Noto Sans SC",
  titleSize: 18,
  legendSize: 18,
  axisSize: 16,
  valueSize: 15,
  emptyTitleSize: 22,
  emptyBodySize: 17,
  donutValueSize: 34,
  percentageSize: 18,
  networkChildSize: 17,
  networkRootSize: 19,
  networkEmptySize: 18,
});

export const TEXT = Object.freeze({
  emptyTitle: "数据不足",
  emptyBody: "当前接口数据不足，暂无法形成该图表。",
  riskTotal: "风险总数",
  networkEmpty: "未取得可展示关系节点",
  quadrantHighImpact: "高影响 / 低紧迫",
  quadrantPriority: "优先整改",
  quadrantObserve: "持续观察",
  quadrantQuick: "快速处理",
});

export const NUMBER_UNITS = Object.freeze({
  tenThousand: 10_000,
  hundredMillion: 100_000_000,
});

export const TITLE_LAYOUT = Object.freeze({
  x: 42,
  y: 43,
  maximumWidth: 330,
});

export const PANEL_LAYOUT = Object.freeze({
  headerHeight: 52,
  titleX: 24,
  titleY: 34,
  titleMaximumWidth: 1080,
  shadowY: 53,
});

export const LEGEND_LAYOUT = Object.freeze({
  left: 420,
  rightPadding: 42,
  top: 20,
  markerSize: 10,
  markerGap: 7,
  itemGap: 20,
  rowHeight: 28,
  baselineOffset: 18,
  bottomOffset: 22,
});

export const LINE_LAYOUT = Object.freeze({
  left: 82,
  right: 1160,
  minimumTop: 100,
  legendGap: 16,
  bottom: 420,
  axisLabelY: 448,
  gridSteps: 5,
  pointRadius: 4,
  lineWidth: 2.5,
});

export const STACKED_BAR_LAYOUT = Object.freeze({
  left: 82,
  right: 1160,
  minimumTop: 160,
  legendGap: 16,
  bottom: 420,
  axisLabelY: 465,
  gridSteps: 5,
  maximumBarWidth: 72,
  minimumBarWidth: 24,
  barWidthRatio: 0.56,
  labelGap: 8,
  minimumLabelWidth: 28,
  minimumCountMaximum: 5,
  tickGap: 12,
  axisTitleX: 25,
  axisTitleY: 300,
  rotatedLabelAngle: -28,
  rotatedLabelMaximumWidth: 92,
  legendLeft: 42,
  legendTop: 103,
  titleCenterX: 600,
  titleCenterY: 86,
  titleMaximumWidth: 600,
});

export const GROUPED_COLUMN_LAYOUT = Object.freeze({
  left: 82,
  right: 1160,
  minimumTop: 105,
  legendGap: 16,
  bottom: 420,
  axisLabelY: 448,
  gridSteps: 5,
  groupWidthRatio: 0.7,
  maximumBarWidth: 42,
  minimumBarWidth: 8,
  barGap: 4,
  labelGap: 8,
  minimumLabelWidth: 28,
  valueGap: 6,
});

export const HORIZONTAL_BAR_LAYOUT = Object.freeze({
  left: 310,
  right: 1125,
  top: 84,
  bottom: 438,
  gridSteps: 7,
  maximumBarHeight: 34,
  barHeightRatio: 0.58,
  labelRightGap: 14,
  labelWidthGap: 42,
  valueGap: 10,
  valueRightPadding: 6,
  valueFlipThreshold: 80,
  labelMinimumFontSize: 11,
  labelMaximumLines: 2,
  labelLineHeightRatio: 1.16,
});

export const DONUT_LAYOUT = Object.freeze({
  centerX: 500,
  centerY: 235,
  outerRadius: 125,
  innerRadius: 75,
  totalValueY: 233,
  totalLabelY: 265,
  legendX: 670,
  legendTop: 125,
  legendMaximumHeight: 330,
  legendMaximumRowHeight: 48,
  legendTextMaximumWidth: 260,
  legendMarkerRadius: 5,
  legendMarkerOffsetY: 5,
  legendTextGap: 18,
  percentageX: 1120,
});

export const PIE_LAYOUT = Object.freeze({
  centerX: 340,
  centerY: 270,
  radius: 150,
});

export const RADAR_LAYOUT = Object.freeze({
  centerX: 430,
  centerY: 270,
  radius: 150,
  gridSteps: 5,
  labelRadius: 190,
  legendLeft: 720,
  legendTop: 120,
  legendRowHeight: 36,
  labelMaximumWidth: 150,
});

export const QUADRANT_LAYOUT = Object.freeze({
  left: 105,
  right: 1110,
  top: 100,
  bottom: 420,
  midpointX: 0.5,
  midpointY: 0.5,
  minimumScaleMaximum: 5,
  pointRadius: 10,
  labelOffsetX: 14,
  labelOffsetY: -10,
  labelMaximumWidth: 180,
  labelBoundaryPadding: 10,
  labelBaselineMinimum: 150,
  labelBaselineBottomPadding: 16,
  labelCollisionGap: 6,
  labelVerticalCandidates: Object.freeze([-10, 14, -34, 38, -58, 62, -82, 86]),
  quadrantPadding: 12,
});

export const EMPTY_LAYOUT = Object.freeze({
  left: 80,
  right: 1120,
  top: 110,
  bottom: 420,
  iconCenterX: 600,
  iconCenterY: 225,
  iconRadius: 34,
  titleY: 300,
  bodyY: 338,
  maximumBodyWidth: 760,
});

export const NETWORK_LAYOUT = Object.freeze({
  maximumChildren: 8,
  rootX: 275,
  rootY: 250,
  rootLeft: 160,
  rootWidth: 230,
  rootHeight: 70,
  rootRadius: 10,
  childCenterX: 835,
  childLeft: 720,
  childWidth: 230,
  childHeight: 50,
  childRadius: 8,
  childTop: 82,
  childRange: 330,
  pathRootX: 390,
  pathControlX: 560,
  pathChildX: 720,
  edgeLabelCenterX: 598,
  edgeLabelLeft: 542,
  edgeLabelWidth: 112,
  edgeLabelHeight: 26,
  edgeLabelRadius: 13,
  nodeTextMaximumWidth: 205,
  nodeMinimumFontSize: 10,
  nodeMaximumLines: 2,
  nodeLineHeightRatio: 1.16,
  emptyTextX: 600,
  emptyTextY: 260,
});

export const TIMELINE_LAYOUT = Object.freeze({
  left: 92,
  right: 1108,
  axisY: 252,
  itemTopY: 118,
  itemBottomY: 340,
  stemLength: 72,
  pointRadius: 7,
  dateOffsetY: 28,
  labelWidth: 176,
  labelMinimumFontSize: 11,
  labelMaximumLines: 3,
  labelLineHeightRatio: 1.18,
  maximumItems: 6,
});

export const LABEL_LAYOUT = Object.freeze({
  framePadding: 8,
  collisionGap: 8,
  axisGap: 16,
  tickGap: 12,
  bottomPadding: 20,
  angles: Object.freeze([0, -45, -60, -90]),
});
