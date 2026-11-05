import { Injectable } from '@angular/core';
import {
    ActivatedRouteSnapshot,
    CanActivate,
    Router,
    RouterStateSnapshot,
} from '@angular/router';
import { AuthService } from '../auth.service';
import { Observable, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';

@Injectable({
    providedIn: 'root',
})
export class AuthGuard implements CanActivate {
    constructor(
        protected authenticationService: AuthService,
        protected router: Router
    ) {}

    canActivate(
        route: ActivatedRouteSnapshot,
        state: RouterStateSnapshot
    ): Observable<any> {
        return this.authenticationService.getFeatures().pipe(
            map(() => {
                return true;
            }),
            catchError(() => {
                this.authenticationService.logout();
                return of(false);
            })
        );
    }
}
