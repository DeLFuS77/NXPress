// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
//
// The threads that compress, called at their start: below the thread that draws (0x2C), so the
// screen stays smooth while they work, as the threads of the install queue (yati.cpp). 0x3B is
// also the one priority the kernel shares out every 10 ms between the threads of a core.
#pragma once

#ifdef __SWITCH__
#include <switch.h>
#endif

namespace nxdock {

// index >= 0: the helper of that number, put on its own core of those this program may use
// (a thread of libnx starts on the core of the process); below 0, it stays where it is.
inline void WorkThread(int index) {
#ifdef __SWITCH__
    svcSetThreadPriority(CUR_THREAD_HANDLE, 0x3B);
    if (index < 0) return;
    u64 mask = 0;
    if (R_FAILED(svcGetInfo(&mask, InfoType_CoreMask, CUR_PROCESS_HANDLE, 0))) return;
    int cores[4], n = 0;
    for (int c = 0; c < 4; c++) {
        if (mask & (1ull << c)) cores[n++] = c;
    }
    if (n == 0) return;
    const int core = cores[index % n];
    svcSetThreadCoreMask(CUR_THREAD_HANDLE, core, 1u << core);
#else
    (void)index;
#endif
}

} // namespace nxdock
