/*
 *
 *     Copyright (C) 2018 ConnectUS
 *
 *     This program is free software: you can redistribute it and/or modify
 *     it under the terms of the GNU General Public License as published by
 *     the Free Software Foundation, either version 3 of the License, or
 *     (at your option) any later version.
 *
 *     This program is distributed in the hope that it will be useful,
 *     but WITHOUT ANY WARRANTY; without even the implied warranty of
 *     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *     GNU General Public License for more details.
 *
 *     You should have received a copy of the GNU General Public License
 *     along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

import * as nodemailer from 'nodemailer';
import * as hbs from 'nodemailer-express-handlebars';
import * as servers from "../server";
import * as fs from "fs";
import * as sanitizeHTML from 'sanitize-html';
import * as path from "path";

export class Mailer {

    public static mailer: Mailer;
    // Use SMTP for now

    public transporter;
    public sender: string; // email to send from

    constructor(username: string, password: string, host: string, port: number, sender: string) {
        this.transporter = nodemailer.createTransport({
            pool: true,
            host: host,
            port: port,
            secure: false,
            auth: {
                user: username,
                pass: password
            }
        });
        this.transporter.verify ((err, success) => {
           if (err) {
               return console.log(err);
           }
           console.log("Verified SMTP configuration!");
        });
        this.transporter.use('compile', hbs({
            viewEngine: 'handlebars',
            viewPath: path.resolve('./templates'),
            extName: '.html'
        }));
    }

    // @ts-ignore
    public async sendSimpleMail(recipient: string, subject: string, text: string, html: string) {
        let mail = {
            from: this.sender,
            to: recipient,
            subject: subject,
            text: text,
            html: html
        };
        try {
            await this.transporter.sendMail(mail);
        } catch (err) {
            throw err;
        }
        if (servers.DEBUG) console.log("Sent mail " + mail.subject);
    }

    // @ts-ignore
    public async sendMail(recipient: string, subject: string, text: string, templateName: string, context) {
        let mail = {
            from: this.sender,
            to: recipient,
            subject: subject,
            text: text,
            template: templateName,
            context: context
        };
        try {
            await this.transporter.sendMail(mail);
        } catch (err) {
            throw err;
        }
        if (servers.DEBUG) console.log("Sent mail " + mail.subject);
    }

    // @ts-ignore
    // NOT USED
    // Retrieve mail template from folder and return it, while replacing the variables and sanitizing the data.
    public static getMailTemplate(replace: Array<[string, string]>, template: string): string {
        let string = fs.readFileSync(__dirname + '/templates/' + template + '.html', 'utf8');
        for (let i = 0; i < replace.length; i++) {
            string = string.replace(new RegExp(("${" + replace[i][0] + "}").replace(/([.*+?^=!:${}()|\[\]\/\\])/g, "\\$1"), 'g'), sanitizeHTML(replace[i][1], {
                allowedTags: [],
                allowedAttributes: {}
            }));
        }
        return string;
    }

}