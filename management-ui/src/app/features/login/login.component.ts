import { Component, OnInit } from '@angular/core';
import { FormBuilder, Validators } from '@angular/forms';
import { AuthService } from '../../core/auth.service';
import { Router } from '@angular/router';
import { first } from 'rxjs/operators';
import packageInfo from '../../../../package.json';

@Component({
    selector: 'app-login',
    templateUrl: './login.component.html',
    styleUrls: ['./login.component.scss'],
    standalone: false,
})
export class LoginComponent implements OnInit {
    public readonly appVersion: string = packageInfo.version;
    form;
    error = '';
    loading = false;

    constructor(
        private fb: FormBuilder,
        private authenticationService: AuthService,
        private router: Router
    ) {
    }

    ngOnInit(): void {
        this.authenticationService.logout();
        this.form = this.fb.group({
            username: ['', Validators.required],
            password: ['', Validators.required]
        });
    }

    submitForm() {
        this.loading = true;
        this.authenticationService.login(this.form.value.username, this.form.value.password)
            .pipe(first())
            .subscribe(
                data => {
                    this.router.navigate(['/servers']);
                    this.loading = false;
                },
                error => {
                    this.error = error;
                    this.loading = false;
                });
    }
}
