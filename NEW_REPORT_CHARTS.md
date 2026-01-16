## **VISUALIZATION #2: Processing Pipeline Waterfall**

### Visual Description
A **horizontal stacked bar chart** that breaks down the total processing time of each LLM model into distinct phases. Each model gets one horizontal bar, subdivided into three segments representing different stages of the inference pipeline. The chart is oriented with model names on the y-axis and time (in milliseconds) on the x-axis.

### Color Palette (Modern, Muted, No Gradients)
- **Prompt Processing**: `#475569` (dark slate gray)
- **First Token Wait**: `#94a3b8` (medium slate gray)
- **Generation Time**: `#cbd5e1` (light slate gray)

This creates a monochromatic scale from dark → light, making it easy to distinguish phases while maintaining a professional appearance.

### Data Sources & Calculations

For each model in the report, extract from `model.accuracy` array (the individual question records):

1. **avgPrompt** (Prompt Processing Time):
   ```javascript
   avgPrompt = average(records.map(r => r.prompt_ms))
   ```
   This is the time spent loading and processing the input prompt.

2. **avgTtft** (Time to First Token):
   ```javascript
   avgTtft = average(records.map(r => r.time_to_first_token))
   ```
   This is the latency before the model outputs its first token.

3. **avgGen** (Generation Time):
   ```javascript
   avgGen = average(records.map(r => r.predicted_ms))
   ```
   This is the time spent generating all output tokens.

**Note**: In the actual data, some models have negative `avgGen` values due to how the metrics are calculated. Use `Math.max(0, avgGen)` to prevent negative bars.

### Chart Configuration

**Type**: Horizontal stacked bar chart (`Chart.js` with `indexAxis: 'y'`)

**Dimensions**:
- Height: 350px (compact)
- Bar thickness: 32px
- Responsive width

**Layout Details**:
- **X-axis** (horizontal): Time in milliseconds, stacked mode enabled
- **Y-axis** (vertical): Model names
- Grid lines: Only on x-axis, color `#f1f5f9` (very subtle)
- Font sizes: Labels 11px, ticks 10px
- Legend: Top position, compact with 12px box width

### Key Features
- **Compact design**: Thinner bars (32px) with minimal padding
- **Clear segmentation**: Three distinct phases clearly visible
- **Tooltip enhancement**: Shows exact milliseconds for each phase
- **No clutter**: Removed y-axis grid, minimal spacing

### Insights This Reveals
1. **Bottleneck identification**: See which phase dominates (e.g., MiniCPM has huge prompt overhead)
2. **Architecture differences**: Some models front-load work (prompt), others spread it out
3. **Total time comparison**: Longer bars = slower overall
4. **Balance assessment**: Ideal models have all three phases relatively short

---

## **VISUALIZATION #3: Model Similarity Dendrogram**

### Visual Description
A **hierarchical clustering tree** (dendrogram) that shows which models exhibit similar performance characteristics. Models are positioned at the "leaves" (endpoints) of the tree, and branches connect models that are similar. The closer two models merge in the tree, the more similar their behavior.

### Visual Structure
- **Orientation**: Left-to-right (root on left, leaves on right)
- **Branch color**: `#94a3b8` (medium slate gray), 2px width
- **Node circles**: 
  - Internal nodes (clusters): `#64748b` (darker gray), 6px radius
  - Leaf nodes (models): `#3b82f6` (blue), 6px radius
- **Labels**: Model/cluster names in `#334155` (slate), 12px font
  - Left-aligned for internal nodes
  - Right-aligned for leaf nodes (models)

### Data Sources & Clustering Logic

The dendrogram is built from a **hierarchical clustering** of models based on their performance metrics. Here's the complete methodology:

#### Step 1: Create Feature Vectors
For each model, create a normalized vector of key metrics:
```javascript
features = {
  accuracy: model.aggregates.accuracy.accuracy,
  throughput: model.aggregates.throughput.avg_tokens_per_second,
  latency: model.aggregates.latency.avg_total_ms,
  stability: model.aggregates.stability.tokens_per_second_cv,
  efficiency: model.aggregates.efficiency.accuracy_per_second
}
```

#### Step 2: Normalize Values
Normalize each metric to 0-1 scale so they're comparable:
```javascript
normalized = (value - min) / (max - min)
```
For latency and stability (where lower is better), invert: `1 - normalized`

#### Step 3: Calculate Distances
Use Euclidean distance between normalized feature vectors:
```javascript
distance = sqrt(
  (acc1 - acc2)² + 
  (tps1 - tps2)² + 
  (lat1 - lat2)² + 
  (stab1 - stab2)² + 
  (eff1 - eff2)²
)
```

#### Step 4: Hierarchical Clustering
Use **agglomerative clustering** (bottom-up):
1. Start with each model as its own cluster
2. Repeatedly merge the two closest clusters
3. Continue until all models are in one tree

**Linkage method**: Average linkage (distance between clusters = average distance between all pairs of points)

### D3.js Implementation

```javascript
// Create hierarchy structure
const hierarchy = d3.hierarchy(clusterData);

// Use d3.cluster() layout
const cluster = d3.cluster()
  .size([height - 100, width - 200]);

cluster(hierarchy);

// Draw links (branches)
svg.selectAll('.link')
  .data(hierarchy.links())
  .join('path')
  .attr('d', d3.linkHorizontal()
    .x(d => d.y)  // Horizontal orientation
    .y(d => d.x))
  .attr('stroke', '#94a3b8')
  .attr('stroke-width', 2);

// Draw nodes
svg.selectAll('.node')
  .data(hierarchy.descendants())
  .join('circle')
  .attr('cx', d => d.y)
  .attr('cy', d => d.x)
  .attr('r', 6)
  .attr('fill', d => d.children ? '#64748b' : '#3b82f6');
```

### Example Hierarchy Structure
```javascript
{
  name: 'All Models',
  children: [
    {
      name: 'Fast Group',
      children: [
        { name: 'Qwen2.5-1.5b', metrics: {...} },
        { name: 'InternLM2.5', metrics: {...} },
        { name: 'DeepSeek-R1', metrics: {...} }
      ]
    },
    {
      name: 'Accurate Group',
      children: [
        { name: 'Gemma-2-2b', metrics: {...} },
        { name: 'Falcon3-3B', metrics: {...} }
      ]
    }
  ]
}
```

### Insights This Reveals
1. **Performance families**: Models with similar architectures/sizes cluster together
2. **Trade-off groups**: Fast models vs. accurate models vs. balanced models
3. **Outliers**: Models that don't cluster well are unique/unusual
4. **Alternative selection**: If your preferred model isn't available, choose from its cluster

---

## **VISUALIZATION #4: Pareto Frontier Enhanced**

### Visual Description
An **enhanced scatter plot** that visualizes the **Pareto frontier** - the set of models that aren't strictly dominated by any other model. The chart plots accuracy (x-axis) vs. throughput (y-axis), with special visual treatment for Pareto-optimal models.

### Visual Elements

1. **Dominated Region** (shaded area):
   - Color: `#10b981` (green) at 10% opacity
   - Fills the area below the Pareto frontier line
   - Represents the "zone of inferiority" where no models exist

2. **Pareto Frontier Line**:
   - Color: `#10b981` (green)
   - Width: 3px
   - Style: Dashed (`stroke-dasharray: '5,5'`)
   - Connects Pareto-optimal models in order of increasing accuracy

3. **Data Points** (circles):
   - **Pareto-optimal models**: `#10b981` (green), 8px radius, bold labels
   - **Dominated models**: `#94a3b8` (gray), 8px radius, normal labels
   - All points: White stroke, 2px width

4. **Labels**:
   - Model names positioned 12px right of points
   - Color: `#334155` (dark slate)
   - Font: 11px, bold for Pareto models

### Data Sources & Pareto Logic

#### Step 1: Determine Pareto Optimality
A model is **Pareto-optimal** if no other model is better in ALL dimensions:

```javascript
function isParetoOptimal(model, allModels) {
  return !allModels.some(other => {
    // Check if 'other' dominates 'model'
    const betterOrEqualAccuracy = other.accuracy >= model.accuracy;
    const betterOrEqualThroughput = other.tps >= model.tps;
    const strictlyBetter = 
      other.accuracy > model.accuracy || 
      other.tps > model.tps;
    
    return betterOrEqualAccuracy && 
           betterOrEqualThroughput && 
           strictlyBetter;
  });
}
```

#### Step 2: Extract Data
For each model:
```javascript
{
  name: model.model,
  accuracy: model.aggregates.accuracy.accuracy * 100,  // Convert to percentage
  tps: model.aggregates.throughput.avg_tokens_per_second,
  pareto: isParetoOptimal(model, allModels)
}
```

#### Step 3: Sort Pareto Points
Sort Pareto-optimal models by accuracy (ascending) to draw the frontier line correctly:
```javascript
const paretoModels = models
  .filter(m => m.pareto)
  .sort((a, b) => a.accuracy - b.accuracy);
```

### D3.js Implementation

```javascript
// Scales
const xScale = d3.scaleLinear()
  .domain([60, 90])  // Accuracy range
  .range([marginLeft, width - marginRight]);

const yScale = d3.scaleLinear()
  .domain([0, 3.5])  // Throughput range
  .range([height - marginBottom, marginTop]);

// Draw dominated region (area below frontier)
const area = d3.area()
  .x(d => xScale(d.accuracy))
  .y0(height - marginBottom)  // Bottom of chart
  .y1(d => yScale(d.tps));     // Frontier line

svg.append('path')
  .datum(paretoModels)
  .attr('d', area)
  .attr('fill', '#10b981')
  .attr('opacity', 0.1);

// Draw frontier line
const line = d3.line()
  .x(d => xScale(d.accuracy))
  .y(d => yScale(d.tps));

svg.append('path')
  .datum(paretoModels)
  .attr('d', line)
  .attr('stroke', '#10b981')
  .attr('stroke-width', 3)
  .attr('stroke-dasharray', '5,5')
  .attr('fill', 'none');

// Draw points
svg.selectAll('.point')
  .data(allModels)
  .join('circle')
  .attr('cx', d => xScale(d.accuracy))
  .attr('cy', d => yScale(d.tps))
  .attr('r', 8)
  .attr('fill', d => d.pareto ? '#10b981' : '#94a3b8')
  .attr('stroke', '#fff')
  .attr('stroke-width', 2);
```

### Axes Configuration
- **X-axis**: Accuracy (%), range 60-90%
- **Y-axis**: Throughput (tokens/sec), range 0-3.5
- **Labels**: 40px below x-axis, 50px left of y-axis
- **Grid**: Subtle, color `#f1f5f9`

### Insights This Reveals
1. **Best trade-offs**: Pareto models offer the best accuracy/speed balance
2. **Dominated models**: Gray models are strictly worse than at least one other
3. **Frontier shape**: Steep = better options; flat = forced trade-offs
4. **Selection guidance**: Only consider green models unless you have specific constraints
5. **Improvement vectors**: Distance from gray to frontier shows potential gains

---

## Implementation Notes

### Data Extraction Pattern
For all three visualizations, you'll need to iterate through the metrics report:

```javascript
const models = metrics.models.filter(m => m.gpu === 'pentium-n3710-1-60ghz');

models.forEach(model => {
  const data = {
    name: model.model,
    accuracy: model.aggregates.accuracy.accuracy * 100,
    tps: model.aggregates.throughput.avg_tokens_per_second,
    avgTotal: model.aggregates.latency.avg_total_ms,
    // ... extract other metrics as needed
  };
});
```

### Libraries Required
- **Visualization #2**: Chart.js 4.4.2+
- **Visualizations #3 & #4**: D3.js v7+

### Responsive Considerations
- All charts should use `width: 100%` and defined heights
- Use `viewBox` for SVG charts to maintain aspect ratios
- Test on mobile: dendrograms especially need horizontal scroll or smaller layouts