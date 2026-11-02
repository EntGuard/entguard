import { Injectable } from '@angular/core';
import { HttpRequest, HttpHandler, HttpEvent, HttpInterceptor } from '@angular/common/http';
import { Observable } from 'rxjs';
import { AuthService } from './auth.service';
import { LoggedUserModel } from '../shared/models/logged-user-model';
import { environment } from '../../environments/environment';

@Injectable()
export class JwtInterceptor implements HttpInterceptor {
    constructor(private authenticationService: AuthService) {}

    intercept(
        request: HttpRequest<any>,
        next: HttpHandler
    ): Observable<HttpEvent<any>> {
        // add authorization header with jwt token if available, only for our own API
        const currentUser: LoggedUserModel =
            this.authenticationService.currentUserValue;
        if (currentUser && currentUser.jwt && request.url.startsWith(environment.REST_BASE_URL)) {
            request = request.clone({
                setHeaders: {
                    Authorization: `Bearer ${currentUser.jwt}`,
                },
            });
        }

        return next.handle(request);
    }
}
