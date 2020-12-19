#include <cassert>
/*! @file
 *  This is an example of the PIN tool that demonstrates some basic PIN APIs 
 *  and could serve as the starting point for developing your first PIN tool
 */

#include "pin.H"
#include <iostream>

using std::cout;
using std::cerr;
using std::string;
using std::endl;

/* ================================================================== */
// Global variables 
/* ================================================================== */

UINT64 memRead;

PIN_LOCK lock;

TLS_KEY bufKey;

typedef VOID (*FLUSHFUNC)(THREADID tid, UINT64 *buf, UINT64 size);

class MemBuffer {
private:
    const UINT64 size;
    const THREADID tid;
    UINT64 *buf;
    UINT64 idx;
    FLUSHFUNC flushFunc;

public:
    MemBuffer(UINT32 size, FLUSHFUNC flushFunc, THREADID tid) : size(size), tid(tid) {
        buf = new UINT64[size];
        idx = 0;
        this->flushFunc = flushFunc;
    }

    ~MemBuffer() {
        this->flushFunc(tid, buf, idx);
        delete buf;
    }

    VOID add(UINT64 addr) {
        buf[idx++] = addr;
        if (idx == size) {
            this->flushFunc(tid, buf, size);
            idx = 0;
        }
    }
};

/* ===================================================================== */
// Command line switches
/* ===================================================================== */

KNOB<UINT32> KnobBufferSize(KNOB_MODE_WRITEONCE, "pintool", "buffersize", "1024",
                            "线程Buffer大小，单位为个64bit整数");

KNOB<bool> KnobBinary(KNOB_MODE_WRITEONCE, "pintool", "binary", "false",
                      "是否使用二进制模式输出");

KNOB<UINT64> KnobStopAt(KNOB_MODE_WRITEONCE, "pintool", "stopat", "50000000000",
                        "内存记录总数，当被追踪进程的内存记录超过这个数时将会停止记录。");

/* ===================================================================== */
// Utilities
/* ===================================================================== */

/*!
 *  Print out help message.
 */
INT32 Usage() {
    cerr << "本工具用于追踪一个进程每一条线程对内存的使用，并输出到标准输出中。" << endl
         << "输出是二进制格式的话，每个数据均为64bit大小。首先将会输出64 bit的threadId，" << endl
         << "然后就是该线程访问过的所有地址。地址输出完毕后，将会输出0作为结束。" << endl;
    cerr << endl << KNOB_BASE::StringKnobSummary() << endl;

    return -1;
}

/* ===================================================================== */
// Analysis routines
/* ===================================================================== */

VOID addMemTrace(THREADID tid, VOID *addr) {
    auto *buf = static_cast<MemBuffer *>(PIN_GetThreadData(bufKey, tid));
    buf->add(reinterpret_cast<UINT64>(addr));
}

/* ===================================================================== */
// Instrumentation callbacks
/* ===================================================================== */

/*!
 * Insert call to the CountBbl() analysis routine before every basic block 
 * of the trace.
 * This function is called every time a new trace is encountered.
 * @param[in]   trace    trace to be instrumented
 * @param[in]   v        value specified by the tool in the TRACE_AddInstrumentFunction
 *                       function call
 */
VOID Trace(TRACE trace, __unused VOID *v) {
    // Visit every basic block in the trace
    UINT64 insCnt = 0;
    for (BBL bbl = TRACE_BblHead(trace); BBL_Valid(bbl); bbl = BBL_Next(bbl)) {
        for (INS ins = BBL_InsHead(bbl); INS_Valid(ins); ins = INS_Next(ins)) {
            INT c = 0;
            if (INS_IsMemoryRead(ins) && INS_IsStandardMemop(ins)) {
                INS_InsertPredicatedCall(ins, IPOINT_BEFORE, (AFUNPTR) addMemTrace, IARG_THREAD_ID, IARG_MEMORYREAD_EA,
                                         IARG_END);
                c++;
            }

            if (INS_HasMemoryRead2(ins) && INS_IsStandardMemop(ins)) {
                INS_InsertPredicatedCall(ins, IPOINT_BEFORE, (AFUNPTR) addMemTrace, IARG_THREAD_ID, IARG_MEMORYREAD2_EA,
                                         IARG_END);
                c++;
            }

            if (INS_IsMemoryWrite(ins) && INS_IsStandardMemop(ins)) {
                INS_InsertPredicatedCall(ins, IPOINT_BEFORE, (AFUNPTR) addMemTrace, IARG_THREAD_ID, IARG_MEMORYWRITE_EA,
                                         IARG_END);
                c++;
            }

            insCnt += c;
        }
    }

    memRead += insCnt;
    if (memRead >= KnobStopAt) {
        PIN_Detach();
    }
}

VOID BufferFull(THREADID tid, UINT64 *buf, UINT64 size) {
    // 只在buffer满的时候获取锁，然后写入到标准输出
    PIN_GetLock(&lock, tid);
    if (KnobBinary) {
        // 二进制模式输出
        cout << UINT64(tid);
        for (UINT64 i = 0; i < size; i++) {
            cout << buf[i];
        }
        cout << UINT64(0);
    } else {
        // 以CSV格式输出
        for (UINT64 i = 0; i < size; i++) {
            cout << tid << ",0x" << std::setw(18) << std::setfill('0') << std::hex << buf[i] << endl;
        }
    }
    PIN_ReleaseLock(&lock);
}

VOID ThreadStart(THREADID threadIndex, __unused CONTEXT *ctxt, __unused INT32 flags, __unused VOID *v) {
    auto *buf = new MemBuffer(KnobBufferSize, BufferFull, threadIndex);
    PIN_SetThreadData(bufKey, buf, threadIndex);
}

VOID ThreadFini(THREADID threadIndex, __unused const CONTEXT *ctxt, __unused INT32 code, __unused VOID *v) {
    auto *s = static_cast<MemBuffer *>(PIN_GetThreadData(bufKey, threadIndex));
    delete s;
    PIN_SetThreadData(bufKey, nullptr, threadIndex);
}


VOID ThreadDetach(THREADID threadIndex, __unused const CONTEXT *ctxt, __unused VOID *v) {
    auto *s = static_cast<MemBuffer *>(PIN_GetThreadData(bufKey, threadIndex));
    delete s;
    PIN_SetThreadData(bufKey, nullptr, threadIndex);
}

/*!
 * The main procedure of the tool.
 * This function is called when the application image is loaded but not yet started.
 * @param[in]   argc            total number of elements in the argv array
 * @param[in]   argv            array of command line arguments, 
 *                              including pin -t <toolname> -- ...
 */
int main(int argc, char *argv[]) {
    // Initialize PIN library. Print help message if -h(elp) is specified
    // in the command line or the command line is invalid 
    if (PIN_Init(argc, argv)) {
        return Usage();
    }

    // 准备数据
    bufKey = PIN_CreateThreadDataKey(nullptr);

    // 注册回调函数
    TRACE_AddInstrumentFunction(Trace, nullptr);
    PIN_AddThreadStartFunction(ThreadStart, nullptr);
    PIN_AddThreadDetachFunction(ThreadDetach, nullptr);
    PIN_AddThreadFiniFunction(ThreadFini, nullptr);

    PIN_StartProgram();

    return 0;
}

/* ===================================================================== */
/* eof */
/* ===================================================================== */
