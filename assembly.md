# 汇编知识集合

## AT&T汇编语言语法总结

### 命令后缀

- `b`: byte = 8 bit
- `w`: word = 16 bit
- `d`: double word
- `q`: quad word
- `l`: double word

### 操作数

- `$`开头为字面值
- `%`开头为寄存器
- 无前缀的数字为内存地址
- 括号包含者内存取值，见下方

### 内存取址

完整格式：`segment-override:signed-offset(base,index,scale)`

#### 样例

|GAS memory operand|NASM memory operand|
|------------------|-------------------|
|100               |[100]              |
|%es:100           |[es:100]           |
|(%eax)            |[eax]              |
|(%eax,%ebx)       |[eax+ebx]          |
|(%ecx,%ebx,2)     |[ecx+ebx*2]        |
|(,%ebx,2)         |[ebx*2]            |
|-10(%eax)         |[eax-10]           |
|%ds:-10(%ebp)     |[ds:ebp-10]        |

## X86_64通用寄存器

|寄存器名称|gcc约定用途|
|---------|---|
|RAX|返回值|
|RBX|被调用者保护*|
|RCX|第4个参数|
|RDX|第3个参数|
|RSI|第2个参数|
|RDI|第1个参数|
|RBP|被调用者保护|
|RSP|栈指针|
|R8 |第5个参数|
|R9 |第6个参数|
|R10|调用者保护*|
|R11|调用者保护|
|R12|被调用者保护|
|R13|被调用者保护|
|R14|被调用者保护|
|R15|被调用者保护|
|R16|被调用者保护|

*调用者保护*：调用函数之前，寄存器若被使用，其值将会被保存到栈帧。调用完之后恢复。

*被调用者保护*：在子函数中，需要使用这个寄存器的，子函数需要保存这个寄存器的值到栈帧，返回前恢复。

## gcc编译

`-S`会让gcc仅编译c代码，不进行汇编，产出后缀为`.s`的汇编代码文件.

## 零碎运行知识

- 运行同一段程序，只要没有随机跳转的代码，得到的控制流是相同的。
- 每秒分析500000条指令，因此最多分析500万指令，否则会花费太多时间

## 利用PT分析内存使用

### 需要做的事情

- [x] 去除跳转指令（无条件跳转jmp以及一系列的有条件跳转）
- [ ] 去除任何试图修改RIP的指令
- [ ] 在发生内存访问时保存访问的地址。不需要完全还原，但是在分析过程中保证前后一致。
- [ ] 若遇到无法编译的指令，则将其转换为访问同位置的内存指令，或相同语义的指令
- [ ] 不允许修改系统保护区块，比如虚拟地址空间中映射为操作系统内存区域的部分
- [ ] 减少一切不会影响寄存器内容的指令，如`cmp`、`test`等，这些指令大多用来控制跳转

### 无法编译指令

#### 错误后缀指令

- `movapsx %xmm0,-0x70(%rbp)`
- `movdqax (%rdi),%xmm2`
- `movdqux (%rax),%xmm4`
- `movupsx (%rsi),%xmm0`
- `pcmpeqbx 0x10(%rax),%xmm1`
- `stosqq (%rdi)`
- `vmovdqay (%rdi),%ymm1`
- `vmovdquy (%rdi),%ymm1`
- `rep stosqq  (%rdi)`
- `paddqx 0x12aada(%rip),%xmm3`
- `stosqq (%rdi)`
- `vpcmpeqby (%rdi),%ymm0,%ymm1`
- `pcmpistrix $0x1a,(%rsi,%rdx,1),%xmm0`
- `vmovdqux (%rsi),%xmm0`
- `palignrx $0xe,-0x10(%rdi,%rdx,1),%xmm0`
- `vpcmpeqby (%rdi),%ymm0,%ymm1`
- `vmovdqux (%rsi),%xmm0`
- `vpalignrx $0x8,-0x10(%rdi,%rdx,1),%xmm0,%xmm0`
- `vmovdqax 0x31159(%rip),%xmm4`
- `cmpsbb (%rdi),(%rsi)`
- `movsqq (%rsi),(%rdi)`

#### 编译出错的指令

- number of operands mismatch for `nop`： 直接删除
- invalid instruction suffix for `movq`： 出错命令为`movqq`，去掉`q`后缀
- invalid instruction suffix for `movlpd`： 出错命令为`movlpdq`，去掉`q`后缀
- invalid instruction suffix for `movhpd`： 出错命令为`movhpdq`，去掉`q`后缀
- invalid instruction suffix for `movhps`： 出错命令为`movhpsq`，去掉`q`后缀

7096.run.s:1096132: Error: no such instruction:

7096.run.s:1314244: Error: invalid instruction suffix for `vmovq'

7096.run.s:1048385: Error: no such instruction:

7096.run.s:1053090: Error: invalid instruction suffix for `movsd'

7096.run.s:1053091: Error: invalid instruction suffix for `addsd'

7096.run.s:1057912: Error: no such instruction:

7096.run.s:1093321: Error: no such instruction: `movsqq (%rsi),(%rdi)'

## 需要对perf进行的修改

#### builtin-script.c

`parse_xed`函数需要删去less参数，以减少对内存的消耗




