import { Component, OnInit } from '@angular/core';
import * as svgIconDetails from '../assets/data/svg-icon-details.json';
import { DomSanitizer } from '@angular/platform-browser';
import { MatIconRegistry } from '@angular/material/icon';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-root',
    templateUrl: './app.component.html',
    styleUrls: ['./app.component.scss'],
    standalone: false,
})
export class AppComponent implements OnInit {
    constructor(
        private sanitizer: DomSanitizer,
        private iconRegistry: MatIconRegistry,
        private translate: TranslateService
    ) {
        this.translate.setDefaultLang('en');
        this.translate.use('en');
    }

    ngOnInit(): void {
        this.registerIcons(<any>svgIconDetails);
    }

    private registerIcons(data: Object): void {
        data['default'].forEach(
            (iconDetail: { iconName: string; svgFilePath: string }) => {
                this.iconRegistry.addSvgIcon(
                    iconDetail.iconName,
                    this.sanitizer.bypassSecurityTrustResourceUrl(
                        iconDetail.svgFilePath
                    )
                );
            }
        );
    }
}
