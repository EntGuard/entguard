import { Injectable } from '@angular/core';
import { MatDialog } from '@angular/material/dialog';
import { MatSnackBar } from '@angular/material/snack-bar';
import { ConfirmDialogComponent } from '../shared/dialogs/delete-dialog/confirm-dialog.component';
import { Observable } from 'rxjs';
import { HttpErrorDialogComponent } from '../shared/dialogs/http-error-dialog/http-error-dialog.component';

@Injectable({
    providedIn: 'root',
})
export class AppUtilsService {
    constructor(
        private matDialog: MatDialog,
        private snackBar: MatSnackBar
    ) { }

    // Show a success message in a snackbar
    showSuccessSnackBar(message: string, duration: number = 3000): void {
        this.snackBar.open(message, 'Close', {
            duration: duration,
            panelClass: ['success-snackbar']
        });
    }

    // Show an error message in a snackbar
    showErrorSnackBar(message: string, duration: number = 5000): void {
        this.snackBar.open(message, 'Close', {
            duration: duration,
            panelClass: ['error-snackbar']
        });
    }

    // Show an info message in a snackbar
    showInfoSnackBar(message: string, duration: number = 3000): void {
        this.snackBar.open(message, 'Close', {
            duration: duration,
            panelClass: ['info-snackbar']
        });
    }

    /**
     * Modifies the value for alphanumerical sorting
     * @param value - any alphanumerical string
     */
    normalizeMixedDataValue(value) {
        const padding = '000000000000000';
        if (value) {
            value = '' + value;
            // Loop over all numeric values in the string and
            // replace them with a value of a fixed-width for
            // both leading (integer) and trailing (decimal)
            // padded zeroes.
            value = value.replace(
                /(\d+)((\.\d+)+)?/g,
                ($0, integer, decimal, $3) => {
                    // If this numeric value has 'multiple'
                    // decimal portions, then the complexity
                    // is too high for this simple approach -
                    // just return the padded integer.
                    if (decimal !== $3) {
                        return (
                            padding.slice(integer.length) + integer + decimal
                        );
                    }
                    decimal = decimal || '.0';
                    return (
                        padding.slice(integer.length) +
                        integer +
                        decimal +
                        padding.slice(decimal.length)
                    );
                }
            );
            return value;
        } else {
            return padding;
        }
    }

    handleHttpError(error) {
        this.matDialog.open(HttpErrorDialogComponent, {
            data: { errorMessage: error },
        });
    }

    showConfirmation(
        message: string,
        actionToBePerformed?: Observable<any>,
        action2ToBePerformed?: Observable<any>,
        warningMessage?: string
    ): Observable<any> {
        return this.matDialog
            .open(ConfirmDialogComponent, {
                data: {
                    confirmMsg: message,
                    warningMsg: warningMessage,
                    actionToBePerformed,
                    action2ToBePerformed,
                },
            })
            .afterClosed();
    }

    generatePresharedKey() {
        const privateKey = new Uint8Array(32);
        window.crypto.getRandomValues(privateKey);
        return privateKey;
    }

    gf(init?: any) {
        let r = new Float64Array(16);
        if (init) {
            for (let i = 0; i < init.length; ++i) r[i] = init[i];
        }
        return r;
    }

    cswap(p, q, b) {
        // tslint:disable-next-line:one-variable-per-declaration
        let t,
            c = ~(b - 1);
        for (let i = 0; i < 16; ++i) {
            t = c & (p[i] ^ q[i]);
            p[i] ^= t;
            q[i] ^= t;
        }
    }

    add(o, a, b) {
        for (var i = 0; i < 16; ++i) o[i] = (a[i] + b[i]) | 0;
    }

    subtract(o, a, b) {
        for (var i = 0; i < 16; ++i) o[i] = (a[i] - b[i]) | 0;
    }

    multmod(o, a, b) {
        let t = new Float64Array(31);
        for (var i = 0; i < 16; ++i) {
            for (var j = 0; j < 16; ++j) t[i + j] += a[i] * b[j];
        }
        for (var i = 0; i < 15; ++i) t[i] += 38 * t[i + 16];
        for (var i = 0; i < 16; ++i) o[i] = t[i];
        this.carry(o);
        this.carry(o);
    }

    carry(o) {
        var c;
        for (var i = 0; i < 16; ++i) {
            o[(i + 1) % 16] += (i < 15 ? 1 : 38) * Math.floor(o[i] / 65536);
            o[i] &= 0xffff;
        }
    }

    generatePublicKey(privateKey) {
        // tslint:disable-next-line:one-variable-per-declaration prefer-const
        let r,
            z = new Uint8Array(32);
        // tslint:disable-next-line:one-variable-per-declaration
        const a = this.gf([1]),
            b = this.gf([9]),
            c = this.gf(),
            d = this.gf([1]),
            e = this.gf(),
            f = this.gf(),
            _121665 = this.gf([0xdb41, 1]),
            _9 = this.gf([9]);
        for (let i = 0; i < 32; ++i) z[i] = privateKey[i];
        this.clamp(z);
        for (let i = 254; i >= 0; --i) {
            r = (z[i >>> 3] >>> (i & 7)) & 1;
            this.cswap(a, b, r);
            this.cswap(c, d, r);
            this.add(e, a, c);
            this.subtract(a, a, c);
            this.add(c, b, d);
            this.subtract(b, b, d);
            this.multmod(d, e, e);
            this.multmod(f, a, a);
            this.multmod(a, c, a);
            this.multmod(c, b, e);
            this.add(e, a, c);
            this.subtract(a, a, c);
            this.multmod(b, a, a);
            this.subtract(c, d, f);
            this.multmod(a, c, _121665);
            this.add(a, a, d);
            this.multmod(c, c, a);
            this.multmod(a, d, f);
            this.multmod(d, b, _9);
            this.multmod(b, e, e);
            this.cswap(a, b, r);
            this.cswap(c, d, r);
        }
        this.invert(c, c);
        this.multmod(a, a, c);
        this.pack(z, a);
        return z;
    }

    pack(o, n) {
        var b,
            m = this.gf(),
            t = this.gf();
        for (let i = 0; i < 16; ++i) t[i] = n[i];
        this.carry(t);
        this.carry(t);
        this.carry(t);
        for (var j = 0; j < 2; ++j) {
            m[0] = t[0] - 0xffed;
            for (var i = 1; i < 15; ++i) {
                m[i] = t[i] - 0xffff - ((m[i - 1] >> 16) & 1);
                m[i - 1] &= 0xffff;
            }
            m[15] = t[15] - 0x7fff - ((m[14] >> 16) & 1);
            b = (m[15] >> 16) & 1;
            m[14] &= 0xffff;
            this.cswap(t, m, 1 - b);
        }
        for (var i = 0; i < 16; ++i) {
            o[2 * i] = t[i] & 0xff;
            o[2 * i + 1] = t[i] >> 8;
        }
    }

    invert(o, i) {
        var c = this.gf();
        for (var a = 0; a < 16; ++a) c[a] = i[a];
        for (var a = 253; a >= 0; --a) {
            this.multmod(c, c, c);
            if (a !== 2 && a !== 4) this.multmod(c, c, i);
        }
        for (var a = 0; a < 16; ++a) o[a] = c[a];
    }

    generatePrivateKey() {
        const privateKey = this.generatePresharedKey();
        this.clamp(privateKey);
        return privateKey;
    }

    encodeBase64(dest, src) {
        const input = Uint8Array.from([
            (src[0] >> 2) & 63,
            ((src[0] << 4) | (src[1] >> 4)) & 63,
            ((src[1] << 2) | (src[2] >> 6)) & 63,
            src[2] & 63,
        ]);
        for (let i = 0; i < 4; ++i)
            dest[i] =
                input[i] +
                65 +
                (((25 - input[i]) >> 8) & 6) -
                (((51 - input[i]) >> 8) & 75) -
                (((61 - input[i]) >> 8) & 15) +
                (((62 - input[i]) >> 8) & 3);
    }

    keyToBase64(key) {
        var i,
            base64 = new Uint8Array(44);
        for (i = 0; i < 32 / 3; ++i)
            this.encodeBase64(base64.subarray(i * 4), key.subarray(i * 3));
        this.encodeBase64(
            base64.subarray(i * 4),
            Uint8Array.from([key[i * 3 + 0], key[i * 3 + 1], 0])
        );
        base64[43] = 61;
        return String.fromCharCode.apply(null, base64);
    }

    clamp(z) {
        z[31] = (z[31] & 127) | 64;
        z[0] &= 248;
    }
}
