import { Injectable } from '@angular/core';
import {
    ActivatedRouteSnapshot,
    CanActivate,
    Router,
    RouterStateSnapshot,
} from '@angular/router';
import { Observable, of } from 'rxjs';
import { AuthService } from '../auth.service';
import { map, mergeMap } from 'rxjs/operators';
import { FeaturesModel } from '../../shared/models/features-model';
import { AuthGuard } from './auth.guard';

@Injectable({
    providedIn: 'root',
})
export class LdapFeatureGuard extends AuthGuard implements CanActivate {
    constructor(authenticationService: AuthService, router: Router) {
        super(authenticationService, router);
    }

    canActivate(
        route: ActivatedRouteSnapshot,
        state: RouterStateSnapshot
    ): Observable<any> {
        return super.canActivate(route, state).pipe(
            mergeMap((authResponse) => {
                // featuresObs already contains desired value because of parent canActivate call
                return this.authenticationService.featuresObs.pipe(
                    map((features: FeaturesModel) => {
                        if (features.ldap) {
                            return authResponse;
                        } else {
                            this.router.navigate(['/premium']);
                            return of(false);
                        }
                    })
                );
            })
        );
    }
}
