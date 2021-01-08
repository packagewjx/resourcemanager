# perf笔记

## 处理器概念

#### Super Queue

In each processor core, there is a super queue that allocates entries to buffer requests of memory access traffic due to an L2 miss to the uncore sub-system. 

#### Demand Read

指定内存地址的读取事件。意味着这是由程序发出的，不是预取指令或硬件预取等发起的。

#### RFO (Reads-For-Ownership)

读取并且独占这一块数据，可以认为这块数据是用于写的。

注意，根据[这里](https://community.intel.com/t5/Software-Tuning-Performance/Difference-between-RFO-requests-and-data-read-requests-to-L2/td-p/1124113)的解答，RFO与Demand Read是相互独立的事件。

####  Uncore

指代非物理核上组件。典型的就是L3 Cache，由一个Socket的CPU核共享。

#### Offcore

指代非逻辑核上组件。像L1和L2 Cache等，但是要注意，一个CPU物理核上的超线程会共享这些Uncore组件。

## Perf事件

| perf事件名                                                   | 固定 | perf描述                                                     | Intel 官方文档描述                                           |
| ------------------------------------------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| instructions<br>inst_retired.any                             | 是   | Instructions retired from execution.                         | Counts the number of X86 instructions retired - an Architectural PerfMon event. Counting continues during hardware interrupts, traps, and inside interrupthandlers. |
| cycles<br>cpu_clk_unhalted.thread                            | 是   | Core cycles when the thread is not in halt state.            | Counts core clock cycles whenever the logical processor is in C0 state (not halted). The frequency of this event varies with state transitions in the core. |
| l1d.replacement                                              |      | L1D data line replacements.                                  | Counts the number of lines brought into the L1 data cache.   |
| L1-dcache-load<br>mem_inst_retired.all_loads                 |      | All retired load instructions.                               | All retired load instructions.                               |
| L1-dcache-stroes<br>mem_inst_retired.all_stores              |      | All retired store instructions.                              | All retired store instructions.                              |
| mem_load_retired.l1_hit                                      |      | Retired load instructions with L1 cache hits as data sources. | **Xeon**<br>Counts retired load instructions with at least one uop Precise event capable. that hit in the L1 data cache. This event includes all SW prefetches and lock instructions regardless of the data source.<br>**Core**<br>Retired load instructions with L1 cache hits as data sources. |
| mem_load_retired.l1_miss                                     |      | Retired load instructions missed L1 cache as data sources.   | **Xeon**<br>Counts retired load instructions with at least one uop that missed in the L1 cache.<br>**Core**<br>Retired load instructions missed L1 cache as data sources. |
| mem_load_retired.fb_hit                                      |      | Retired load instructions which data sources were load missed L1 but hit FB due to preceding miss to the same cache line with data not ready. | **Xeon**<br>Counts retired load instructions with at least one uop was load missed in L1 but hit FB (Fill Buffers) due to preceding miss to the same cache line with data not ready.<br>**Core**<br>Retired load instructions where data sources were load PSDLA uops missed L1 but hit FB due to preceding miss to the same cache line with data not ready. |
| mem_load_retired.l2_hit                                      |      | Retired load instructions with L2 cache hits as data sources. | Retired load instructions with L2 cache hits as data sources. |
| mem_load_retired.l2_miss                                     |      | Retired load instructions missed L2 cache as data sources.   | Retired load instructions missed L2. Unknown data source excluded. |
| mem_load_retired.l3_hit                                      |      | Retired load instructions with L3 cache hits as data sources. | Retired load instructions with L3 cache hits as data sources. |
| mem_load_retired.l3_miss                                     |      | Retired load instructions missed L3 cache as data sources.   | Retired load instructions missed L3. Excludes unknown data source. |
| l2_rqsts.miss                                                |      | All requests that miss L2 cache.                             | All requests that missed L2.                                 |
| l2_rqsts.references                                          |      | All L2 requests.                                             | All requests to L2.                                          |
| l2_rqsts.all_demand_data_rd                                  |      | Demand Data Read requests                                    |                                                              |
| l2_rqsts.demand_data_rd_hit                                  |      | Demand Data Read requests that hit L2 cache.                 |                                                              |
| l2_rqsts.demand_data_rd_miss                                 |      | Demand Data Read miss L2, no rejects.                        |                                                              |
| cache-misses<br>longest_lat_cache.miss                       |      | Core-originated cacheable demand requests missed L3.         | **Xeon**<br>Counts core-originated cacheable requests to the L3 cache (Longest Latency cache). Requests include data and code reads, Reads-for-Ownership (RFOs), speculative accesses and hardware prefetches from L1 and L2. It does not include all accesses to the L3.<br>**Core**<br>This event counts each cache miss condition for references to the L3 cache. |
| cache-references<br>longest_lat_cache.reference              |      | Core-originated cacheable demand requests that refer to L3.  | **Xeon**<br>Counts core-originated cacheable requests to the L3 cache (Longest Latency cache). Requests include data and code reads, Reads-for-Ownership (RFOs), speculative accesses and hardware prefetches from L1 and L2. It does not include all accesses to the L3.<br>**Core**<br>This event counts requests originating from the core that reference a cache line in the L3 cache. |
| l1d_pend_miss.pending                                        |      | L1D miss outstandings duration in cycles.                    | **Xeon**<br>Counts duration of L1D miss outstanding, that is each cycle number of Fill Buffers (FB) outstanding required by Demand Reads. FB either is held by demand loads, or it is held by non-demand loads and gets hit at least once by demand. The valid outstanding interval is defined until the FB deallocation by one of the following ways: from FB allocation, if FB is allocated by demand from the demand Hit FB, if it is allocated by hardware or software prefetch.Note: In the L1D, a Demand Read contains cacheable or noncacheable demand loads, including ones causing cache-line splits and reads due to page walks resulted from any request type.<br>**Core**<br>Increments the number of outstanding L1D misses every cycle. |
| cycle_activity.cycles_l1d_miss                               |      | Cycles while L1 cache miss demand load is outstanding.       | Cycles while L1 data cache miss demand load is outstanding.  |
| cycle_activity.stalls_l1d_miss                               |      | Execution stalls while L1 cache miss demand load is outstanding. | Execution stalls while L1 data cache miss demand load is outstanding. |
| cycle_activity.cycles_l2_miss                                |      | Cycles while L2 cache miss demand load is outstanding.       | Cycles while L2 cache miss demand load is outstanding.       |
| cycle_activity.stalls_l2_miss                                |      | Execution stalls while L2 cache miss demand load is outstanding. | Execution stalls while L2 cache miss demand load is outstanding. |
| cycle_activity.cycles_l3_miss                                |      | Cycles while L3 cache miss demand load is outstanding.       | Cycles while L3 cache miss demand load is outstanding.       |
| cycle_activity.stalls_l3_miss                                |      | Execution stalls while L3 cache miss demand load is outstanding. | Execution stalls while L3 cache miss demand load is outstanding. |
| cycle_activity.cycles_mem_any                                |      | Cycles while memory subsystem has an outstanding load.       | Cycles while memory subsystem has an outstanding load.       |
| cycle_activity.stalls_mem_any                                |      | Execution stalls while memory subsystem has an outstanding load. | Execution stalls while memory subsystem has an outstanding load. |
| offcore_requests_outstanding.cycles_with_data_rd             |      | Cycles when offcore outstanding cacheable Core Data Read transactions are present in SuperQueue (SQ), queue to uncore. |                                                              |
| offcore_requests.all_data_rd                                 |      | Demand and prefetch data reads.                              |                                                              |
| offcore_requests.all_requests                                |      | Any memory transaction that reached the SQ.                  | **Xeon**<br>Counts memory transactions reached the super queue including requests initiated by the core, all L3 prefetches, page walks, etc.<br>**Core**<br>Any memory transaction that reached the SQ. |
| offcore_requests_outstanding.cycles_with_l3_miss_demand_data_rd |      | Cycles with at least 1 Demand Data Read requests who miss L3 cache in the superQ | Cycles with at least one offcore outstanding demand data read requests from SQ that missed L3. |
| offcore_requests.l3_miss_demand_data_rd                      |      | Demand Data Read requests who miss L3 cache                  | Demand data read requests that missed L3.                    |
| offcore_requests.demand_data_rd                              |      | Demand Data Read requests sent to uncore                     | Counts the Demand Data Read requests sent to uncore. Use it in conjunction with OFFCORE_REQUESTS_OUTSTANDING to determine average latency in the uncore. |
表格说明：

- Intel官方描述把Xeon与Core相差较大的分开列出。
- Intel官方描述中Core指代第6、7、8代微处理器架构（Skylake, Kaby Lake and Coffee Lake）。
- “固定”是CPU微架构中包含的固定计数器，仅能用于计数一个事件，不会占用PMU，因此能够能与PMU事件共存，而不丢失采样精度。
- Intel官方手册的事件名通常perf事件名一样，只不过是全部大写。

## 指标之间的关系

- `mem_inst_retired.all_loads = mem_load_retired.l1_hit + mem_load_retired.fb_hit + mem_load_retired.l1_miss + `
- `mem_load_retired.l1_miss = mem_load_retired.l2_hit + mem_load_retired.l2_miss`
- `mem_load_retired.l2_miss = mem_load_retired.l3_hit + mem_load_retired.l3_miss`

### 未知关系

- `longest_lat_cache.reference > offcore_requests.all_requests`
- `offcore_requests.l3_miss_demand_data_rd > mem_load_retired.l3_miss`
- `offcore_requests.demand_data_rd > mem_load_retired.l2_miss`

## 指标用法

#### 测量内存总访问

`result = mem_inst_retired.all_loads + mem_inst_retired.all_stores`

#### 测量内存访问延迟

`result = offcore_requests_outstanding.cycles_with_l3_miss_demand_data_rd / offcore_requests.l3_miss_demand_data_rd`

#### 测量L3 Miss Penalty

这个结果比上面的大，可能包含了其他一些未知的延迟

`result = cycle_activity.cycles_l3_miss / offcore_requests.l3_miss_demand_data_rd `

#### 测量全缓存Miss Rate

`result = longest_lat_cache.miss / (mem_inst_retired.all_loads + mem_inst_retired.all_stores) `

#### 测量L3 Miss Rate

针对仅访问访问L3的请求

`result = longest_lat_cache.miss / longest_lat_cache.reference`

#### 测量全缓存Hit的平均访问周期

暂且使用Demand Load的数据来测量，目前不清楚`mem_inst_retired.all_loads`是否都是Demand Load。

````
hit_cycles = cycle_activity.cycles_mem_any - cycle_activity.cycles_l3_miss
hit_instuctions = mem_inst_retired.all_loads - mem_load_retired.l3_miss
average_cycles_per_hit_instruction = hit_cycles / hit_instructions
````



