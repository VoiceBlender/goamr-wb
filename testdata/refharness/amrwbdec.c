/*
 * Reference decoder harness for the differential tests: stock opencore-amrwb
 * (Apache-2.0) driven through its public D_IF_* API.
 *
 *   amrwbdec          stdin: frames as [uint16 LE length][storage frame:
 *                     ToC byte + speech bytes], so mode and frame type can
 *                     change per frame.
 *   amrwbdec <mode>   stdin: packed speech bytes only, fixed mode 0..8.
 *
 * stdout: 320 int16 LE samples per frame.
 */
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>

/* opencore-amrwb/dec_if.h, declared here so only the runtime library is needed. */
void *D_IF_init(void);
void D_IF_decode(void *state, const unsigned char *in, short *out, int bfi);
void D_IF_exit(void *state);

static const int speech_bytes[9] = {17, 23, 32, 36, 40, 46, 50, 58, 60};

int main(int argc, char **argv) {
    void *d = D_IF_init();
    uint8_t f[64];
    int16_t out[320];

    if (argc > 1) {
        int mode = atoi(argv[1]);
        if (mode < 0 || mode > 8) {
            fprintf(stderr, "mode must be 0..8\n");
            return 2;
        }
        int n = speech_bytes[mode];
        f[0] = (uint8_t)(mode << 3 | 0x04);
        while (fread(f + 1, 1, n, stdin) == (size_t)n) {
            D_IF_decode(d, f, out, 0);
            fwrite(out, 2, 320, stdout);
        }
    } else {
        uint8_t len[2];
        while (fread(len, 1, 2, stdin) == 2) {
            int n = len[0] | len[1] << 8;
            if (n < 1 || n > (int)sizeof f || fread(f, 1, n, stdin) != (size_t)n) {
                fprintf(stderr, "bad frame length %d\n", n);
                return 1;
            }
            D_IF_decode(d, f, out, 0);
            fwrite(out, 2, 320, stdout);
        }
    }
    D_IF_exit(d);
    return 0;
}
