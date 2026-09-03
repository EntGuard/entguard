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
            catchError((error: Response) => {
                if (error.status === 401 || error.status === 403) {
                    this.router.navigate(['/login']);
                } else {
                    // todo: implement some UX error handling
                    console.error(error);
                }
                this.authenticationService.logout();
                return of(false);
            })
        );

        // not logged in so redirect to login page with the return url
        // this.router.navigate(['/login'], { queryParams: { returnUrl: state.url } });
        // return of(false);
    }
}
