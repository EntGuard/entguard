
-- +migrate Up

/*
Update password of default admin account to safer values (but only if the account credentials have not been updated yet)
New password is '5Bt3kp0ItQ;9' (hashed using bcrypt with cost 13)
Old password is 'pwadmin' (hashed using bcrypt with cost 4)
*/

UPDATE users
SET password = '$2a$13$xNvFykZxN/loi7X7CcSAnuFF3avCzNfPs0cBweKh5a97mi0vhUnx6'
WHERE username = 'admin'
    AND password = '$2y$04$DK8gh3iNbm1eHpbi.M7./utqHO4eDbb8wytwKgqHh3/b6JajQfG0K'
    AND is_admin = TRUE;

-- +migrate Down

UPDATE users
SET password = '$2y$04$DK8gh3iNbm1eHpbi.M7./utqHO4eDbb8wytwKgqHh3/b6JajQfG0K'
WHERE username = 'admin'
    AND password = '$2a$13$xNvFykZxN/loi7X7CcSAnuFF3avCzNfPs0cBweKh5a97mi0vhUnx6'
    AND is_admin = TRUE;
