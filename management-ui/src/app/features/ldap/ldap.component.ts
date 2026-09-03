import { Component, OnDestroy, OnInit } from '@angular/core';
import { TranslateService } from '@ngx-translate/core';
import { Subject } from 'rxjs';
import { takeUntil } from 'rxjs/operators';

@Component({
    selector: 'app-ldap',
    templateUrl: './ldap.component.html',
    styleUrls: ['./ldap.component.scss'],
    standalone: false,
})
export class LdapComponent implements OnInit, OnDestroy {
    private componentDestroyed: Subject<void> = new Subject<void>();
    navLinks = [
        {
            label: 'ldapconfigs-list.headerLdapSyncStatus',
            link: 'synchronization-status',
        },
        {
            label: 'ldapconfigs-list.headerLdapConfigs',
            link: 'configurations',
        },
        {
            label: 'ldapconfigs-list.headerLdapTemplates',
            link: 'templates',
        },
    ];

    constructor(private translateService: TranslateService) { }

    ngOnInit(): void {
        this.translateLabels();
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    private translateLabels(): void {
        this.translateService.stream('ldapconfigs-list').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            this.navLinks.forEach(navLink => {
                if (navLink.label) {
                    navLink.label = this.translateService.instant(navLink.label);
                }
            });
        });
    }
}
