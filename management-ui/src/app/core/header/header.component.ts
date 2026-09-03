import { Component, OnDestroy, OnInit } from '@angular/core';
import { AuthService } from '../auth.service';
import { FeaturesModel } from '../../shared/models/features-model';
import { Subject } from 'rxjs';
import { takeUntil } from 'rxjs/operators';

@Component({
    selector: 'app-header',
    templateUrl: './header.component.html',
    styleUrls: ['./header.component.scss'],
    standalone: false,
})
export class HeaderComponent implements OnInit, OnDestroy {
    loggedUser: any;
    features: FeaturesModel;
    private componentDestroyed: Subject<void> = new Subject<void>();

    constructor(public authService: AuthService) { }

    ngOnInit(): void {
        this.authService.currentUser.subscribe((user) => {
            this.loggedUser = user;
        });
        this.authService.featuresObs
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe((features) => (this.features = features));
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    logOut() {
        this.authService.logout();
    }
}
