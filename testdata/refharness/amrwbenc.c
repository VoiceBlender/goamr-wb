/*
 * Reference encoder harness for the differential tests: stock vo-amrwbenc
 * (Apache-2.0) driven through its public E_IF_* API.
 *
 *   amrwbenc <mode>   stdin: 320 int16 LE samples per frame, fixed mode 0..8.
 *   amrwbenc          stdin: per frame, one mode byte (0..8) then 320 int16 LE
 *                     samples, so the mode can change per frame. Bit 7 of the
 *                     mode byte turns DTX on for that frame.
 *
 * stdout: storage frames (ToC byte + speech or SID bytes; ToC only for
 * NO_DATA).
 */
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>

/* vo-amrwbenc/enc_if.h, declared here so only the runtime library is needed. */
void *E_IF_init(void);
int E_IF_encode(void *state, int mode, const short *speech, unsigned char *out, int dtx);
void E_IF_exit(void *state);

int main(int argc, char **argv) {
    int mode = -1;
    if (argc > 1) {
        mode = atoi(argv[1]);
        if (mode < 0 || mode > 8) {
            fprintf(stderr, "mode must be 0..8\n");
            return 2;
        }
    }
    void *e = E_IF_init();
    int16_t in[320];
    uint8_t out[64];
    for (;;) {
        int m = mode, dtx = 0;
        if (m < 0) {
            uint8_t b;
            if (fread(&b, 1, 1, stdin) != 1)
                break;
            dtx = b >> 7;
            b &= 0x7f;
            if (b > 8) {
                fprintf(stderr, "bad mode byte %d\n", b);
                return 1;
            }
            m = b;
        }
        if (fread(in, 2, 320, stdin) != 320)
            break;
        int n = E_IF_encode(e, m, in, out, dtx);
        fwrite(out, 1, n, stdout);
    }
    E_IF_exit(e);
    return 0;
}
