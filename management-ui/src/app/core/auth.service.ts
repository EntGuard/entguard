import { Injectable } from '@angular/core';
import { environment } from 'src/environments/environment';
import { BehaviorSubject, Observable } from 'rxjs';
import { HttpClient } from '@angular/common/http';
import { map, tap } from 'rxjs/operators';
import { LoggedUserModel } from '../shared/models/logged-user-model';
import { DataModelsService } from './data-models.service';
import { Router } from '@angular/router';
import { FeaturesModel } from '../shared/models/features-model';

@Injectable({
    providedIn: 'root',
})
export class AuthService extends DataModelsService {
    private currentUserSubject: BehaviorSubject<LoggedUserModel>;
    public currentUser: Observable<LoggedUserModel>;
    private featuresSubj: BehaviorSubject<FeaturesModel> =
        new BehaviorSubject<FeaturesModel>(new FeaturesModel());
    public featuresObs = this.featuresSubj.asObservable();

    constructor(private http: HttpClient, private router: Router) {
        super(http);
        this.currentUserSubject = new BehaviorSubject<LoggedUserModel>(
            JSON.parse(localStorage.getItem('currentUser'))
        );
        this.currentUser = this.currentUserSubject.asObservable();
    }

    public get currentUserValue(): LoggedUserModel {
        return this.currentUserSubject.value;
    }

    patchPassword(userId: string, password: string): Observable<void> {
        return this.patch(`users/${userId}/password`, { password });
    }

    login(username: string, password: string): Observable<LoggedUserModel> {
        return this.http
            .post<LoggedUserModel>(`${environment.REST_BASE_URL}jwt`, {
                username,
                password,
            })
            .pipe(
                map((user: LoggedUserModel) => {
                    // store user details and jwt token in local storage to keep user logged in between page refreshes
                    user['username'] = username;
                    localStorage.setItem('currentUser', JSON.stringify(user));
                    this.currentUserSubject.next(user);
                    return user;
                })
            );
    }

    getFeatures(): Observable<any> {
        return this.getOneModel('features', FeaturesModel).pipe(
            tap((response: FeaturesModel) => this.featuresSubj.next(response))
        );
    }

    logout(): void {
        localStorage.removeItem('currentUser');
        this.currentUserSubject.next(null);
        this.router.navigate(['/login']);
    }
}
